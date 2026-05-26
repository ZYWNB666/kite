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
	"sync"
	"time"
)

const feishuBase = "https://open.feishu.cn/open-apis"

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
	resp, err := http.Post(feishuBase+"/auth/v3/app_access_token/internal", "application/json", bytes.NewReader(b))
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

	resp, err := http.DefaultClient.Do(req)
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

	resp, err := http.DefaultClient.Do(req)
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

// VerifyCardSignature verifies the X-Lark-Signature header for card action callbacks.
// Feishu signature = lowercase_hex(sha256(timestamp + nonce + verification_token + body)).
func VerifyCardSignature(timestamp, nonce, token, body, signature string) bool {
	content := timestamp + nonce + token + body
	hash := sha256.Sum256([]byte(content))
	expected := hex.EncodeToString(hash[:])
	return expected == signature
}

// BuildRequestCard creates the interactive card payload for a new access request.
func BuildRequestCard(requestID uint, requesterName, cluster, namespace string, durationHours int, reason, approverOpenID, approverName string) map[string]interface{} {
	durationStr := formatDuration(durationHours)
	atMention := fmt.Sprintf(`<at id="%s">%s</at>`, approverOpenID, approverName)
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"template": "blue",
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "🔐 Kite 权限申请",
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
							"content": fmt.Sprintf("**集群**\n`%s`", cluster),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**命名空间**\n`%s`", namespace),
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
						"is_short": false,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**申请原因**\n%s", reason),
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("申请编号 #%d · 审批人: %s", requestID, atMention),
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
							"request_id": fmt.Sprintf("%d", requestID),
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
							"request_id": fmt.Sprintf("%d", requestID),
						},
					},
				},
			},
		},
	}
}

// BuildResultCard creates the card to replace the action buttons after approval/rejection.
func BuildResultCard(requestID uint, requesterName, cluster, namespace string, durationHours int, reason, approverName, status, note string) map[string]interface{} {
	durationStr := formatDuration(durationHours)
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
	default:
		template = "grey"
		statusText = status
	}

	elements := []interface{}{
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
						"content": fmt.Sprintf("**集群**\n`%s`", cluster),
					},
				},
				map[string]interface{}{
					"is_short": true,
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**命名空间**\n`%s`", namespace),
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
					"is_short": false,
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**申请原因**\n%s", reason),
					},
				},
			},
		},
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
				"content": fmt.Sprintf("申请编号 #%d · 审批人: %s", requestID, approverName),
			},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"template": template,
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": fmt.Sprintf("🔐 Kite 权限申请 · %s", statusText),
			},
		},
		"elements": elements,
	}
}

// BuildReminderCard creates a reminder card for a pending request.
func BuildReminderCard(requestID uint, requesterName, cluster, namespace string, durationHours int, reason, approverOpenID, approverName string) map[string]interface{} {
	card := BuildRequestCard(requestID, requesterName, cluster, namespace, durationHours, reason, approverOpenID, approverName)
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
