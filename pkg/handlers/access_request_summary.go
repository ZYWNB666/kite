package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zxh326/kite/pkg/ai"
	"github.com/zxh326/kite/pkg/feishu"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

// summarizeAndNotifyAccessUsage collects the user's operation history during the
// approved access period, asks AI to summarize it (including high-risk detection),
// and posts the summary as a Feishu card reply in the original request card's thread.
//
// This function is designed to be called asynchronously (go summarizeAndNotifyAccessUsage(req)).
// It silently returns if: AI is not configured, no operation records exist,
// or the Feishu bot is not configured / has no message_id to reply to.
func summarizeAndNotifyAccessUsage(req *model.AccessRequest) {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("access_request summary: panic for request #%d: %v", req.ID, r)
		}
	}()

	// 1. Collect operation history
	histories, err := collectAccessUsageHistory(req)
	if err != nil {
		klog.Warningf("access_request summary: failed to query history for request #%d: %v", req.ID, err)
		return
	}
	if len(histories) == 0 {
		klog.Infof("access_request summary: no operations for request #%d, skipping", req.ID)
		return
	}

	// 2. Build stats summary
	stats := buildUsageStats(histories)

	// 3. Call AI for analysis
	cfg, err := ai.LoadRuntimeConfig()
	if err != nil || cfg == nil || !cfg.Enabled {
		klog.Infof("access_request summary: AI not enabled, skipping for request #%d", req.ID)
		return
	}

	agent, err := ai.NewAgent(nil, cfg)
	if err != nil {
		klog.Warningf("access_request summary: failed to create AI agent for request #%d: %v", req.ID, err)
		return
	}

	systemPrompt := `你是 Kubernetes 安全审计助手。请根据用户在临时权限期间的操作记录，生成一份简洁的使用总结。

要求：
1. 概述用户执行了哪些类型的操作（创建、更新、删除等）及其数量
2. 明确标注是否存在高危操作，包括但不限于：
   - 删除操作（delete）
   - 涉及 kube-system 等系统命名空间的操作
   - AI 触发的自动操作
   - 失败的操作
3. 如果存在高危操作，请在开头用 ⚠️ 标注
4. 用中文回复，简洁明了，不要超过 500 字`

	userMessage := buildUsagePrompt(req, histories, stats)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	summary, err := agent.SimpleChat(ctx, systemPrompt, userMessage)
	if err != nil {
		klog.Warningf("access_request summary: AI call failed for request #%d: %v", req.ID, err)
		return
	}

	if strings.TrimSpace(summary) == "" {
		klog.Infof("access_request summary: AI returned empty summary for request #%d, skipping", req.ID)
		return
	}

	// 4. Send summary card to Feishu thread
	sendSummaryCard(req, summary, stats)
}

// collectAccessUsageHistory queries ResourceHistory records for the access request's
// user, cluster, namespace(s), and time range (approved → expired).
func collectAccessUsageHistory(req *model.AccessRequest) ([]model.ResourceHistory, error) {
	query := model.DB.Model(&model.ResourceHistory{}).
		Where("operator_id = ?", req.RequesterID).
		Where("cluster_name = ?", req.Cluster)

	// Namespace may be comma-separated for multi-namespace requests
	nsList := strings.Split(req.Namespace, ",")
	for i, ns := range nsList {
		nsList[i] = strings.TrimSpace(ns)
	}
	if len(nsList) == 1 {
		query = query.Where("namespace = ?", nsList[0])
	} else {
		query = query.Where("namespace IN ?", nsList)
	}

	// Time range: from approval time to expiry time
	startTime := req.ApprovedAt
	if startTime == nil {
		// Fallback: use UpdatedAt (which was set during approval save)
		startTime = &req.UpdatedAt
	}
	endTime := req.ExpiresAt
	if endTime == nil {
		// Fallback: if no ExpiresAt, use current time
		now := time.Now()
		endTime = &now
	}
	query = query.Where("created_at BETWEEN ? AND ?", *startTime, *endTime)

	var histories []model.ResourceHistory
	if err := query.Order("created_at ASC").Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}

// buildUsageStats creates a concise statistics string from the operation history.
func buildUsageStats(histories []model.ResourceHistory) string {
	total := len(histories)
	opCount := map[string]int{}  // operation type → count
	srcCount := map[string]int{} // operation source → count
	failCount := 0
	resources := map[string]bool{} // unique "kind/name" → true

	for _, h := range histories {
		opCount[h.OperationType]++
		srcCount[h.OperationSource]++
		if !h.Success {
			failCount++
		}
		resources[fmt.Sprintf("%s/%s", h.ResourceType, h.ResourceName)] = true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("总操作数：%d\n", total))
	sb.WriteString(fmt.Sprintf("涉及资源数：%d\n", len(resources)))

	// Operation breakdown
	sb.WriteString("操作类型：")
	parts := make([]string, 0, len(opCount))
	for _, op := range []string{"create", "update", "patch", "delete", "apply"} {
		if c, ok := opCount[op]; ok {
			parts = append(parts, fmt.Sprintf("%s %d", op, c))
		}
	}
	for op, c := range opCount {
		if op != "create" && op != "update" && op != "patch" && op != "delete" && op != "apply" {
			parts = append(parts, fmt.Sprintf("%s %d", op, c))
		}
	}
	sb.WriteString(strings.Join(parts, "、"))
	sb.WriteString("\n")

	// Source breakdown
	if aiCount, ok := srcCount["ai"]; ok && aiCount > 0 {
		sb.WriteString(fmt.Sprintf("AI 操作：%d 次\n", aiCount))
	}

	if failCount > 0 {
		sb.WriteString(fmt.Sprintf("失败操作：%d 次", failCount))
	}

	return sb.String()
}

// buildUsagePrompt constructs the user message for the AI, including a concise
// summary of each operation (without full YAML to save tokens).
func buildUsagePrompt(req *model.AccessRequest, histories []model.ResourceHistory, stats string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("用户 %s 获得了对集群 %s 命名空间 %s 的临时访问权限。\n",
		req.RequesterName, req.Cluster, req.Namespace))
	sb.WriteString(fmt.Sprintf("授权时长：%d 小时\n\n", req.DurationHours))
	sb.WriteString(fmt.Sprintf("操作统计：\n%s\n\n", stats))
	sb.WriteString("操作明细：\n")

	// Limit to 50 operations to control prompt length
	maxShow := 50
	for i, h := range histories {
		if i >= maxShow {
			sb.WriteString(fmt.Sprintf("... 还有 %d 条操作记录省略\n", len(histories)-maxShow))
			break
		}
		status := "成功"
		if !h.Success {
			status = "失败"
		}
		src := ""
		if h.OperationSource == "ai" {
			src = " [AI]"
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s %s/%s (ns: %s)%s — %s\n",
			i+1,
			h.CreatedAt.Format("01-02 15:04"),
			h.OperationType,
			h.ResourceType,
			h.ResourceName,
			h.Namespace,
			src,
			status,
		))
	}

	return sb.String()
}

// sendSummaryCard builds and sends the AI summary card as a thread reply.
func sendSummaryCard(req *model.AccessRequest, summary, stats string) {
	bot, setting, err := getFeishuBot()
	if err != nil || bot == nil || req.MessageID == "" || setting.GroupChatID == "" {
		klog.Infof("access_request summary: feishu not configured or no message_id for request #%d, skipping", req.ID)
		return
	}

	card := feishu.BuildSummaryCard(
		req.ID,
		req.RequesterName,
		req.Cluster,
		req.Namespace,
		req.DurationHours,
		summary,
		stats,
	)

	if err := bot.ReplyCard(req.MessageID, card); err != nil {
		klog.Warningf("access_request summary: failed to reply card for request #%d: %v", req.ID, err)
		return
	}

	klog.Infof("access_request summary: sent summary card for request #%d", req.ID)
}
