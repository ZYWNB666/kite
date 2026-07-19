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

var (
	collectAccessUsageHistoryForSummary   = collectAccessUsageHistory
	generateAccessUsageAnalysisForSummary = generateAccessUsageAnalysis
	persistAccessUsageSummaryForSummary   = model.SaveAccessSummaryContent
	sendAccessUsageSummaryCard            = sendSummaryCard
	sendAISummaryFailureForSummary        = sendAISummaryFailureNotice
	sendMissingMessageIDForSummary        = sendMissingMessageIDNotice
)

const maxAccessSummaryAIAttempts = 3

type accessSummaryResult struct {
	MessageID      string
	FailedNotified bool
	FailureReason  string
	AIAttempts     int
}

type accessSummaryAIError struct {
	reason string
}

func (e *accessSummaryAIError) Error() string { return e.reason }

type accessSummaryDeliveryError struct {
	err error
}

func (e *accessSummaryDeliveryError) Error() string { return e.err.Error() }
func (e *accessSummaryDeliveryError) Unwrap() error { return e.err }

type accessSummaryNotificationError struct {
	reason     string
	aiAttempts int
	err        error
}

func (e *accessSummaryNotificationError) Error() string {
	return fmt.Sprintf("send summary failure notification: %v", e.err)
}

func (e *accessSummaryNotificationError) Unwrap() error { return e.err }

// generateAndSendAccessUsageSummary builds and sends one durable summary job.
// Zero-change requests receive an explicit summary. AI failures are retried
// separately and reported to the original request thread after three attempts.
// Delivery errors are returned so the database-backed queue can retry them.
func generateAndSendAccessUsageSummary(req *model.AccessRequest) (result accessSummaryResult, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("access_request summary: panic for request #%d: %v", req.ID, r)
			resultErr = fmt.Errorf("summary panic: %v", r)
		}
	}()

	if req.MessageID == "" {
		reason := fmt.Sprintf("申请 #%d 缺少原始飞书 message_id，无法将权限使用总结回复到对应申请线程", req.ID)
		messageID, err := sendMissingMessageIDForSummary(req, reason)
		if err != nil {
			return accessSummaryResult{}, &accessSummaryNotificationError{
				reason: reason, aiAttempts: req.SummaryAIAttempts, err: err,
			}
		}
		return accessSummaryResult{
			MessageID: messageID, FailedNotified: true, FailureReason: reason, AIAttempts: req.SummaryAIAttempts,
		}, nil
	}
	if req.SummaryContent != "" {
		return sendPersistedAccessUsageSummary(req, req.SummaryContent, req.SummaryStats)
	}

	// 1. Collect operation history
	histories, err := collectAccessUsageHistoryForSummary(req)
	if err != nil {
		return accessSummaryResult{}, fmt.Errorf("query history: %w", err)
	}

	stats := buildUsageStats(histories)
	if len(histories) == 0 {
		summary := buildNoChangeAccessUsageSummary()
		return persistAndSendAccessUsageSummary(req, summary, stats)
	}

	if req.SummaryAIAttempts >= maxAccessSummaryAIAttempts {
		return notifyAISummaryFailure(req, req.SummaryLastError, req.SummaryAIAttempts)
	}

	summary, aiErr := generateAccessUsageAnalysisForSummary(req, histories, stats)
	if aiErr != nil {
		reason := conciseSummaryFailureReason(aiErr)
		attempts := req.SummaryAIAttempts + 1
		if attempts < maxAccessSummaryAIAttempts {
			return accessSummaryResult{}, &accessSummaryAIError{reason: reason}
		}
		return notifyAISummaryFailure(req, reason, attempts)
	}

	return persistAndSendAccessUsageSummary(req, summary, stats)
}

func persistAndSendAccessUsageSummary(req *model.AccessRequest, summary, stats string) (accessSummaryResult, error) {
	updated, err := persistAccessUsageSummaryForSummary(req.ID, req.SummaryClaimToken, summary, stats)
	if err != nil {
		return accessSummaryResult{}, fmt.Errorf("persist generated summary: %w", err)
	}
	if !updated {
		return accessSummaryResult{}, fmt.Errorf("persist generated summary: claim is no longer active")
	}
	req.SummaryContent = summary
	req.SummaryStats = stats
	return sendPersistedAccessUsageSummary(req, summary, stats)
}

func sendPersistedAccessUsageSummary(req *model.AccessRequest, summary, stats string) (accessSummaryResult, error) {
	messageID, err := sendAccessUsageSummaryCard(req, summary, stats)
	if err != nil {
		return accessSummaryResult{}, &accessSummaryDeliveryError{err: err}
	}
	return accessSummaryResult{MessageID: messageID, AIAttempts: req.SummaryAIAttempts}, nil
}

func buildNoChangeAccessUsageSummary() string {
	return "授权期间未检测到资源变更操作。用户可能仅进行了查看资源、查看日志等只读操作；当前总结不会将“无资源变更”解释为“未使用权限”。"
}

// generateAccessUsageAnalysis asks AI for a detailed analysis. Failures are
// returned so the durable worker can retry AI independently up to three times.
func generateAccessUsageAnalysis(req *model.AccessRequest, histories []model.ResourceHistory, stats string) (string, error) {
	cfg, err := ai.LoadRuntimeConfig()
	if err != nil {
		return "", fmt.Errorf("加载 AI 配置失败: %w", err)
	}
	if cfg == nil || !cfg.Enabled {
		return "", fmt.Errorf("AI 未启用")
	}

	agent, err := ai.NewAgent(nil, cfg)
	if err != nil {
		return "", fmt.Errorf("创建 AI Agent 失败: %w", err)
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
		return "", fmt.Errorf("AI 调用失败: %w", err)
	}

	if strings.TrimSpace(summary) == "" {
		return "", fmt.Errorf("AI 返回了空内容")
	}

	return summary, nil
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
	endTime := req.EndedAt
	if endTime == nil {
		endTime = req.ExpiresAt
	}
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
	if len(parts) == 0 {
		sb.WriteString("无资源变更")
	} else {
		sb.WriteString(strings.Join(parts, "、"))
	}
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

func conciseSummaryFailureText(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return "未知错误"
	}
	runes := []rune(reason)
	if len(runes) > 500 {
		return string(runes[:500]) + "…"
	}
	return reason
}

func conciseSummaryFailureReason(err error) string {
	if err == nil {
		return "未知错误"
	}
	return conciseSummaryFailureText(err.Error())
}

func notifyAISummaryFailure(req *model.AccessRequest, reason string, aiAttempts int) (accessSummaryResult, error) {
	reason = conciseSummaryFailureText(reason)
	messageID, err := sendAISummaryFailureForSummary(req, reason, aiAttempts)
	if err != nil {
		return accessSummaryResult{}, &accessSummaryNotificationError{
			reason: reason, aiAttempts: aiAttempts, err: err,
		}
	}
	return accessSummaryResult{
		MessageID: messageID, FailedNotified: true, FailureReason: reason, AIAttempts: aiAttempts,
	}, nil
}

func sendAISummaryFailureNotice(req *model.AccessRequest, reason string, aiAttempts int) (string, error) {
	bot, _, err := getFeishuBot()
	if err != nil {
		return "", fmt.Errorf("load feishu bot: %w", err)
	}
	if bot == nil {
		return "", fmt.Errorf("feishu bot is disabled")
	}
	text := fmt.Sprintf("⚠️ Kite 权限使用总结失败\n申请编号：#%d\n申请人：%s\n集群：%s\n命名空间：%s\nAI 已尝试：%d 次\n失败原因：%s\n请管理员检查 AI 配置或服务状态。",
		req.ID, req.RequesterName, req.Cluster, req.Namespace, aiAttempts, reason)
	messageID, err := bot.ReplyTextWithMessageID(req.MessageID, text)
	if err != nil {
		return "", fmt.Errorf("reply AI failure notice: %w", err)
	}
	return messageID, nil
}

func sendMissingMessageIDNotice(req *model.AccessRequest, reason string) (string, error) {
	bot, setting, err := getFeishuBot()
	if err != nil {
		return "", fmt.Errorf("load feishu bot: %w", err)
	}
	if bot == nil || setting.GroupChatID == "" {
		return "", fmt.Errorf("feishu bot is disabled or group chat is not configured")
	}
	text := fmt.Sprintf("⚠️ Kite 权限总结投递失败\n申请编号：#%d\n申请人：%s\n集群：%s\n命名空间：%s\n失败原因：%s\n请检查原始申请卡片发送记录。",
		req.ID, req.RequesterName, req.Cluster, req.Namespace, reason)
	messageID, err := bot.SendText(setting.GroupChatID, text)
	if err != nil {
		return "", fmt.Errorf("send missing message_id notice: %w", err)
	}
	return messageID, nil
}

// buildUsagePrompt constructs the user message for the AI, including the full
// operation details with YAML diffs so the AI can describe what changed.
func buildUsagePrompt(req *model.AccessRequest, histories []model.ResourceHistory, stats string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("用户 %s 获得了对集群 %s 命名空间 %s 的临时访问权限。\n",
		req.RequesterName, req.Cluster, req.Namespace))
	sb.WriteString(fmt.Sprintf("授权时长：%d 小时\n\n", accessRequestGrantedHours(req)))
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

func accessRequestGrantedHours(req *model.AccessRequest) int {
	endTime := req.EndedAt
	if endTime == nil {
		endTime = req.ExpiresAt
	}
	if req.ApprovedAt == nil || endTime == nil || !endTime.After(*req.ApprovedAt) {
		return req.DurationHours
	}
	duration := endTime.Sub(*req.ApprovedAt)
	return int((duration + time.Hour - 1) / time.Hour)
}

// sendSummaryCard posts to the original request thread. The caller handles a
// missing message ID by sending a plain-text failure alert, never a summary
// card. After repeated reply failures for an existing message ID, this falls
// back to a standalone card so a deleted original message cannot block forever.
func sendSummaryCard(req *model.AccessRequest, summary, stats string) (string, error) {
	bot, setting, err := getFeishuBot()
	if err != nil {
		return "", fmt.Errorf("load feishu bot: %w", err)
	}
	if bot == nil {
		return "", fmt.Errorf("feishu bot is disabled")
	}

	card := feishu.BuildSummaryCard(
		req.ID,
		req.RequesterName,
		req.Cluster,
		req.Namespace,
		accessRequestGrantedHours(req),
		summary,
		stats,
	)

	if req.MessageID == "" {
		return "", fmt.Errorf("request #%d has no message_id", req.ID)
	}

	messageID, err := bot.ReplyCardWithMessageID(req.MessageID, card)
	if err == nil {
		return messageID, nil
	}
	if req.SummaryDeliveryAttempts < 2 {
		return "", fmt.Errorf("reply summary card: %w", err)
	}
	if setting.GroupChatID == "" {
		return "", fmt.Errorf("reply summary card: %v; standalone fallback unavailable: group chat is not configured", err)
	}

	klog.Warningf("access_request summary: thread reply failed repeatedly for request #%d, falling back to group card: %v", req.ID, err)
	messageID, fallbackErr := bot.SendCard(setting.GroupChatID, card)
	if fallbackErr != nil {
		return "", fmt.Errorf("reply summary card: %v; standalone fallback: %w", err, fallbackErr)
	}
	return messageID, nil
}
