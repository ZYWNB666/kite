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
		startStr := "nil"
		if req.ApprovedAt != nil {
			startStr = req.ApprovedAt.Format("01-02 15:04:05")
		}
		endStr := "nil"
		if req.ExpiresAt != nil {
			endStr = req.ExpiresAt.Format("01-02 15:04:05")
		}
		klog.Infof("access_request summary: no operations for request #%d (user=%d, cluster=%s, ns=%s, range=[%s, %s]), skipping",
			req.ID, req.RequesterID, req.Cluster, req.Namespace, startStr, endStr)
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

	systemPrompt := `你是 Kubernetes 安全审计助手。请根据用户在临时权限期间的操作记录，生成一份详细的使用总结。

要求：
1. 概述用户执行了哪些类型的操作（创建、更新、删除等）及其数量
2. 详细描述每个操作的具体内容：
   - 对于 create/apply 操作：说明创建了什么资源，关键配置（镜像、副本数、端口、环境变量、资源限制等）
   - 对于 update/patch 操作：对比变更前后的 YAML，明确指出具体改了哪些字段（如镜像版本、副本数、配置项等）
   - 对于 delete 操作：说明删除了什么资源
3. 明确标注是否存在高危操作，包括但不限于：
   - 删除操作（delete）
   - 涉及 model-serving、envoy-gateway-system、kube-system等命名空间的操作
   - AI 触发的自动操作
   - 失败的操作
   - 修改了安全相关字段（如 securityContext、hostNetwork、privileged 等）
4. 如果存在高危操作，请在开头用 ⚠️ 标注
5. 用中文回复，内容要详尽完整，不要省略操作细节`

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
		// Fallback for old data (ApprovedAt was added later):
		// Reconstruct approval time as ExpiresAt - DurationHours.
		// This is more reliable than UpdatedAt which gets overwritten on every save.
		if req.ExpiresAt != nil && req.DurationHours > 0 {
			t := req.ExpiresAt.Add(-time.Duration(req.DurationHours) * time.Hour)
			startTime = &t
		} else {
			// Last resort: use CreatedAt (request submission time)
			t := req.CreatedAt
			startTime = &t
		}
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

// buildUsagePrompt constructs the user message for the AI, including the full
// operation details with YAML diffs so the AI can describe what changed.
func buildUsagePrompt(req *model.AccessRequest, histories []model.ResourceHistory, stats string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("用户 %s 获得了对集群 %s 命名空间 %s 的临时访问权限。\n",
		req.RequesterName, req.Cluster, req.Namespace))
	sb.WriteString(fmt.Sprintf("授权时长：%d 小时\n\n", req.DurationHours))
	sb.WriteString(fmt.Sprintf("操作统计：\n%s\n\n", stats))
	sb.WriteString("操作明细（含变更内容）：\n")

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
			if h.ErrorMessage != "" {
				status = fmt.Sprintf("失败: %s", h.ErrorMessage)
			}
		}
		src := ""
		if h.OperationSource == "ai" {
			src = " [AI]"
		}
		sb.WriteString(fmt.Sprintf("\n--- 操作 %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("时间: %s\n", h.CreatedAt.Format("01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("操作: %s\n", h.OperationType))
		sb.WriteString(fmt.Sprintf("资源: %s/%s (命名空间: %s)%s\n", h.ResourceType, h.ResourceName, h.Namespace, src))
		sb.WriteString(fmt.Sprintf("结果: %s\n", status))

		// Include YAML content for create/apply/update/patch operations
		if h.OperationType == "create" || h.OperationType == "apply" {
			if h.ResourceYAML != "" {
				sb.WriteString("创建的资源配置:\n")
				sb.WriteString(h.ResourceYAML)
				sb.WriteString("\n")
			}
		} else if h.OperationType == "update" || h.OperationType == "patch" {
			if h.PreviousYAML != "" {
				sb.WriteString("变更前配置:\n")
				sb.WriteString(h.PreviousYAML)
				sb.WriteString("\n")
			}
			if h.ResourceYAML != "" {
				sb.WriteString("变更后配置:\n")
				sb.WriteString(h.ResourceYAML)
				sb.WriteString("\n")
			}
		} else if h.OperationType == "delete" {
			if h.PreviousYAML != "" {
				sb.WriteString("被删除的资源配置:\n")
				sb.WriteString(h.PreviousYAML)
				sb.WriteString("\n")
			}
		}
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
