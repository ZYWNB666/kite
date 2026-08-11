// Package feishu provides a lightweight Feishu/Lark bot client used for
// sending interactive card messages and verifying card-action callbacks.
package feishu

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const feishuBase = "https://open.feishu.cn/open-apis"

var botHTTPClient = &http.Client{Timeout: 30 * time.Second}

// BotClient holds Feishu app credentials and caches the app_access_token.
type BotClient struct {
	AppID     string
	AppSecret string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewBotClient creates a new BotClient.
func NewBotClient(appID, appSecret string) *BotClient {
	return &BotClient{AppID: appID, AppSecret: appSecret}
}

// GetAppAccessToken returns a valid app_access_token, refreshing when necessary.
func (c *BotClient) GetAppAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	body := map[string]string{"app_id": c.AppID, "app_secret": c.AppSecret}
	b, _ := json.Marshal(body)
	resp, err := botHTTPClient.Post(feishuBase+"/auth/v3/app_access_token/internal", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("feishu get token: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
		Expire         int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("feishu decode token response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu get token: code=%d msg=%s", result.Code, result.Msg)
	}
	c.token = result.AppAccessToken
	ttl := result.Expire - 60
	if ttl < 60 {
		ttl = 60
	}
	c.tokenExp = time.Now().Add(time.Duration(ttl) * time.Second)
	return c.token, nil
}

// SendCard sends an interactive card message to a chat and returns the message_id.
func (c *BotClient) SendCard(chatID string, card map[string]interface{}) (string, error) {
	token, err := c.GetAppAccessToken()
	if err != nil {
		return "", err
	}
	cardJSON, _ := json.Marshal(card)
	payload := map[string]string{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost,
		feishuBase+"/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := botHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu send card: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("feishu decode send response: %w, body=%s", err, string(respBody))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu send card: code=%d msg=%s body=%s", result.Code, result.Msg, string(respBody))
	}
	return result.Data.MessageID, nil
}

// PatchCard updates the content of an existing interactive card message.
func (c *BotClient) PatchCard(messageID string, card map[string]interface{}) error {
	token, err := c.GetAppAccessToken()
	if err != nil {
		return err
	}
	cardJSON, _ := json.Marshal(card)
	payload := map[string]string{"content": string(cardJSON)}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPatch,
		feishuBase+"/im/v1/messages/"+messageID,
		bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := botHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("feishu patch card: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("feishu decode patch response: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu patch card: code=%d msg=%s", result.Code, result.Msg)
	}
	return nil
}

// ReplyCard replies to an existing message (by message_id) with an interactive card.
// The reply is posted in the thread/topic of the original message.
func (c *BotClient) ReplyCard(messageID string, card map[string]interface{}) error {
	_, err := c.ReplyCardWithMessageID(messageID, card)
	return err
}

// ReplyCardWithMessageID replies to an existing message and returns the ID of
// the new reply. The ID lets durable callers record successful delivery.
func (c *BotClient) ReplyCardWithMessageID(messageID string, card map[string]interface{}) (string, error) {
	token, err := c.GetAppAccessToken()
	if err != nil {
		return "", err
	}
	cardJSON, _ := json.Marshal(card)
	payload := map[string]interface{}{
		"msg_type":        "interactive",
		"content":         string(cardJSON),
		"reply_in_thread": true,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost,
		feishuBase+"/im/v1/messages/"+messageID+"/reply",
		bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := botHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu reply card: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("feishu decode reply response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu reply card: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data.MessageID, nil
}

// VerifyCardSignature verifies the X-Lark-Signature header for card action callbacks.
// Feishu signature = lowercase_hex(sha256(timestamp + nonce + verification_token + body)).
func VerifyCardSignature(timestamp, nonce, token, body, signature string) bool {
	content := timestamp + nonce + token + body
	hash := sha256.Sum256([]byte(content))
	expected := hex.EncodeToString(hash[:])
	return expected == signature
}

// ReplyText sends a text message as a thread reply to an existing message.
func (c *BotClient) ReplyText(messageID, text string) error {
	_, err := c.ReplyTextWithMessageID(messageID, text)
	return err
}

// ReplyTextWithMessageID replies with text and returns the new reply ID.
func (c *BotClient) ReplyTextWithMessageID(messageID, text string) (string, error) {
	token, err := c.GetAppAccessToken()
	if err != nil {
		return "", err
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]interface{}{
		"msg_type":        "text",
		"content":         string(content),
		"reply_in_thread": true,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost,
		feishuBase+"/im/v1/messages/"+messageID+"/reply",
		bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := botHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu reply text: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("feishu decode reply response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu reply text: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data.MessageID, nil
}

// SendText sends a standalone text message to a chat and returns its message ID.
func (c *BotClient) SendText(chatID, text string) (string, error) {
	token, err := c.GetAppAccessToken()
	if err != nil {
		return "", err
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]string{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost,
		feishuBase+"/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := botHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu send text: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("feishu decode send text response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu send text: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data.MessageID, nil
}

// RequestCardData holds all optional fields for building richer request cards.
type RequestCardData struct {
	RequestID       uint
	RequesterName   string
	Cluster         string
	Namespace       string
	RequestType     string // "full_update" | "canary_update" | "route_adjust" | ""
	ReportLink      string
	TargetResources []string
	DurationHours   int
	RiskLevel       string
	Reason          string
	ApproverOpenID  string
	ApproverName    string
}

// BuildRequestCardFromData creates the interactive card payload for a new access request.
func BuildRequestCardFromData(d RequestCardData) map[string]interface{} {
	durationStr := formatDuration(d.DurationHours)
	riskStr := formatRiskLevel(d.RiskLevel)
	atMention := fmt.Sprintf(`<at id="%s">%s</at>`, d.ApproverOpenID, d.ApproverName)

	titleSuffix, template, noteText := requestCardTypeInfo(d.RequestType)

	fields := []interface{}{
		map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**申请人**\n%s", d.RequesterName),
			},
		},
		map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**集群**\n%s", d.Cluster),
			},
		},
		map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**命名空间**\n%s", d.Namespace),
			},
		},
		map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**申请时长**\n%s", durationStr),
			},
		},
		map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**预估风险**\n%s", riskStr),
			},
		},
	}

	// Add report link as full-width field if present
	if d.ReportLink != "" {
		linkLabel := "🔗 测试报告"
		if d.RequestType == "full_update" {
			linkLabel = "🔗 灰度测试报告"
		}
		fields = append(fields, map[string]interface{}{
			"is_short": false,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**%s**\n%s", linkLabel, d.ReportLink),
			},
		})
	}

	// Add target resources for route_adjust
	if len(d.TargetResources) > 0 {
		fields = append(fields, map[string]interface{}{
			"is_short": false,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**目标配置**\n%s", strings.Join(d.TargetResources, "、")),
			},
		})
	}

	fields = append(fields, map[string]interface{}{
		"is_short": false,
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": fmt.Sprintf("**申请原因**\n%s", d.Reason),
		},
	})

	elements := []interface{}{
		map[string]interface{}{
			"tag":    "div",
			"fields": fields,
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("申请编号 #%d · 审批人: %s%s", d.RequestID, atMention, noteText),
				},
			},
		},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":  "button",
					"type": "primary",
					"text": map[string]interface{}{
						"tag":     "plain_text",
						"content": "✅ 批准",
					},
					"value": map[string]interface{}{
						"action":     "approve",
						"request_id": fmt.Sprintf("%d", d.RequestID),
					},
				},
				map[string]interface{}{
					"tag":  "button",
					"type": "danger",
					"text": map[string]interface{}{
						"tag":     "plain_text",
						"content": "❌ 拒绝",
					},
					"value": map[string]interface{}{
						"action":     "reject",
						"request_id": fmt.Sprintf("%d", d.RequestID),
					},
				},
			},
		},
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
			"update_multi":     true,
		},
		"header": map[string]interface{}{
			"template": template,
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": fmt.Sprintf("🔐 Kite 权限申请%s", titleSuffix),
			},
		},
		"elements": elements,
	}
}

// BuildRequestCard creates the interactive card payload for a new access request.
// Kept for backward compatibility — delegates to BuildRequestCardFromData.
func BuildRequestCard(requestID uint, requesterName, cluster, namespace string, durationHours int, riskLevel, reason, approverOpenID, approverName string) map[string]interface{} {
	return BuildRequestCardFromData(RequestCardData{
		RequestID: requestID, RequesterName: requesterName, Cluster: cluster,
		Namespace: namespace, DurationHours: durationHours, RiskLevel: riskLevel,
		Reason: reason, ApproverOpenID: approverOpenID, ApproverName: approverName,
	})
}

// requestCardTypeInfo returns title suffix, template color, and note suffix per request type.
func requestCardTypeInfo(requestType string) (titleSuffix, template, noteSuffix string) {
	switch requestType {
	case "full_update":
		return "· 全量更新", "blue", ""
	case "canary_update":
		return "· 灰度更新", "turquoise", ""
	case "route_adjust":
		return "· 路由调整", "violet", " · 仅配置增改查（禁删除）"
	default:
		return "", "blue", ""
	}
}

// BuildResultCard creates the card to replace the action buttons after approval/rejection.
func BuildResultCard(requestID uint, requesterName, cluster, namespace string, durationHours int, riskLevel, reason, approverName, status, note string) map[string]interface{} {
	return BuildResultCardFromData(RequestCardData{
		RequestID: requestID, RequesterName: requesterName, Cluster: cluster,
		Namespace: namespace, DurationHours: durationHours, RiskLevel: riskLevel,
		Reason: reason, ApproverName: approverName,
	}, status, note)
}

// BuildResultCardFromData creates the result card with full request type support.
func BuildResultCardFromData(d RequestCardData, status, note string) map[string]interface{} {
	titleSuffix, _, _ := requestCardTypeInfo(d.RequestType)
	var template, statusText string
	switch status {
	case "approved":
		template = "green"
		statusText = "✅ 已批准"
	case "rejected":
		template = "red"
		statusText = "❌ 已拒绝"
	case "withdrawn":
		template = "grey"
		statusText = "↩️ 已撤回"
	case "expired":
		template = "grey"
		statusText = "⏰ 已过期"
	default:
		template = "grey"
		statusText = status
	}

	// Reuse the request card as the single source of truth for labels, field order,
	// links and request-type-specific content. A result card only changes the
	// status header and replaces the pending note/actions with the result details.
	card := BuildRequestCardFromData(d)
	requestElements, _ := card["elements"].([]interface{})
	elements := make([]interface{}, 0, len(requestElements))
	for _, element := range requestElements {
		elementMap, ok := element.(map[string]interface{})
		if !ok {
			elements = append(elements, element)
			continue
		}
		if tag, _ := elementMap["tag"].(string); tag == "note" || tag == "action" {
			continue
		}
		elements = append(elements, element)
	}
	if note != "" {
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**审批意见**\n%s", note),
			},
		})
	}
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{
				"tag":     "plain_text",
				"content": fmt.Sprintf("申请编号 #%d · 审批人: %s", d.RequestID, d.ApproverName),
			},
		},
	})
	card["header"] = map[string]interface{}{
		"template": template,
		"title": map[string]interface{}{
			"tag":     "plain_text",
			"content": fmt.Sprintf("🔐 Kite 权限申请%s · %s", titleSuffix, statusText),
		},
	}
	card["elements"] = elements
	return card
}

// BuildReminderCard creates a reminder card for a pending request.
func BuildReminderCard(requestID uint, requesterName, cluster, namespace string, durationHours int, riskLevel, reason, approverOpenID, approverName string) map[string]interface{} {
	card := BuildRequestCard(requestID, requesterName, cluster, namespace, durationHours, riskLevel, reason, approverOpenID, approverName)
	// Update header to indicate it's a reminder
	card["header"] = map[string]interface{}{
		"template": "orange",
		"title": map[string]interface{}{
			"tag":     "plain_text",
			"content": "🔔 Kite 权限申请（催办）",
		},
	}
	return card
}

// BuildReminderCardFromData creates a reminder card with full request type support.
func BuildReminderCardFromData(d RequestCardData) map[string]interface{} {
	card := BuildRequestCardFromData(d)
	card["header"] = map[string]interface{}{
		"template": "orange",
		"title": map[string]interface{}{
			"tag":     "plain_text",
			"content": "🔔 Kite 权限申请（催办）",
		},
	}
	return card
}

// FormatDuration returns a human-readable Chinese duration string (exported).
func FormatDuration(hours int) string {
	return formatDuration(hours)
}

func formatDuration(hours int) string {
	if hours < 24 {
		return fmt.Sprintf("%d 小时", hours)
	}
	days := hours / 24
	remaining := hours % 24
	if remaining == 0 {
		return fmt.Sprintf("%d 天", days)
	}
	return fmt.Sprintf("%d 天 %d 小时", days, remaining)
}

func formatRiskLevel(level string) string {
	switch level {
	case "low":
		return "🟢 低"
	case "medium":
		return "🟡 中"
	case "high":
		return "🔴 高"
	default:
		return level
	}
}

// BuildSummaryCard creates an interactive card for an AI-generated access usage summary.
// It is posted as a thread reply to the original request card.
func BuildSummaryCard(requestID uint, requesterName, cluster, namespace string, durationHours int, summary string, stats string) map[string]interface{} {
	durationStr := formatDuration(durationHours)
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
			"update_multi":     true,
		},
		"header": map[string]interface{}{
			"template": "grey",
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "📋 权限使用总结",
			},
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"fields": []interface{}{
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**申请人**\n%s", requesterName),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**集群**\n%s", cluster),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**命名空间**\n%s", namespace),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**授权时长**\n%s", durationStr),
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**操作统计**\n%s", stats),
				},
			},
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**使用分析**\n%s", summary),
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("申请编号 #%d · 权限已过期", requestID),
					},
				},
			},
		},
	}
}

// BuildExpiringSoonCard creates an interactive card notifying that a permission
// is about to expire, with renewal buttons. Posted as a thread reply.
func BuildExpiringSoonCard(requestID uint, requesterName, cluster, namespace string, expiresAt string) map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
			"update_multi":     true,
		},
		"header": map[string]interface{}{
			"template": "orange",
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "⏰ 权限即将到期",
			},
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"fields": []interface{}{
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**申请人**\n%s", requesterName),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**集群**\n%s", cluster),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**命名空间**\n%s", namespace),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**到期时间**\n%s", expiresAt),
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": "⚠️ 权限将在 10 分钟后到期，如需继续使用请审批续期：",
				},
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag":  "button",
						"type": "primary",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "续期 1h",
						},
						"value": map[string]interface{}{
							"action":     "renew",
							"request_id": fmt.Sprintf("%d", requestID),
							"hours":      "1",
						},
					},
					map[string]interface{}{
						"tag":  "button",
						"type": "primary",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "续期 2h",
						},
						"value": map[string]interface{}{
							"action":     "renew",
							"request_id": fmt.Sprintf("%d", requestID),
							"hours":      "2",
						},
					},
					map[string]interface{}{
						"tag":  "button",
						"type": "primary",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "续期 4h",
						},
						"value": map[string]interface{}{
							"action":     "renew",
							"request_id": fmt.Sprintf("%d", requestID),
							"hours":      "4",
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("申请编号 #%d · 审批人或管理员可操作续期", requestID),
					},
				},
			},
		},
	}
}
