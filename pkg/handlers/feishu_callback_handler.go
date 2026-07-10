package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/feishu"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

// feishuCBAction is the card action payload (same shape in old and new formats).
type feishuCBAction struct {
	Tag    string                 `json:"tag"`
	Value  map[string]interface{} `json:"value"`
	OpenID string                 `json:"open_id"` // old format only
}

// feishuCBOperator is the operator info (same shape in old and new formats).
type feishuCBOperator struct {
	OpenID string `json:"open_id"`
	UserID string `json:"user_id"`
}

func callbackValueToString(v interface{}) (string, bool) {
	switch vv := v.(type) {
	case string:
		if vv == "" {
			return "", false
		}
		return vv, true
	case float64:
		return strconv.FormatInt(int64(vv), 10), true
	case int:
		return strconv.Itoa(vv), true
	case int64:
		return strconv.FormatInt(vv, 10), true
	default:
		return "", false
	}
}

func isApproverMatched(expectedUID string, candidates ...string) bool {
	if expectedUID == "" {
		return false
	}
	for _, c := range candidates {
		if c != "" && c == expectedUID {
			return true
		}
	}
	return false
}

// isConfiguredApprover checks whether the operator is in the configured approver list.
func isConfiguredApprover(setting *model.FeishuNotificationSetting, candidates ...string) bool {
	approvers := setting.GetApprovers()
	if len(approvers) == 0 {
		// No approver list configured — allow anyone (open mode)
		return true
	}
	approverSet := make(map[string]bool, len(approvers))
	for _, a := range approvers {
		approverSet[a.OpenID] = true
	}
	for _, c := range candidates {
		if c != "" && approverSet[c] {
			return true
		}
	}
	return false
}

// feishuCardCallback handles both old (card.action.trigger_v1) and new (card.action.trigger v2.0) formats,
// as well as URL verification challenges.
type feishuCardCallback struct {
	// URL verification (event-subscription style challenge)
	Type      string `json:"type"`      // "url_verification"
	Challenge string `json:"challenge"` // challenge value to echo back

	// Old format fields (card.action.trigger_v1)
	Token    string           `json:"token"` // VerificationToken at root
	Action   feishuCBAction   `json:"action"`
	Operator feishuCBOperator `json:"operator"`

	// New v2.0 format fields (card.action.trigger)
	Schema string `json:"schema"` // "2.0"
	Header struct {
		EventID   string `json:"event_id"`
		Token     string `json:"token"`      // VerificationToken
		EventType string `json:"event_type"` // "card.action.trigger"
	} `json:"header"`
	Event struct {
		Operator feishuCBOperator `json:"operator"`
		Action   feishuCBAction   `json:"action"`
	} `json:"event"`
}

// HandleFeishuCardCallback handles POST /api/feishu/card-callback.
// This is a public endpoint called directly by Feishu servers.
func HandleFeishuCardCallback(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	var cb feishuCardCallback
	if err := json.Unmarshal(bodyBytes, &cb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	// Handle URL verification challenge FIRST — no auth needed for this handshake.
	if cb.Type == "url_verification" {
		c.JSON(http.StatusOK, gin.H{"challenge": cb.Challenge})
		return
	}

	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		klog.Warningf("feishu callback: failed to load setting: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "configuration error"})
		return
	}

	// Security: Verification Token comparison (official "Verification Token 校验" approach).
	// Per docs, signature verification (X-Lark-Signature) uses EncryptKey — a separate credential.
	// We use the simpler token-in-body comparison which matches what users configure here.
	if storedToken := string(setting.VerificationToken); storedToken != "" {
		if cb.Schema == "2.0" {
			if cb.Header.Token != storedToken {
				klog.Warningf("feishu callback: token mismatch from %s (schema=2.0)", c.ClientIP())
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
		} else if cb.Token != "" && cb.Token != storedToken {
			// 旧版卡片回调字段语义与事件订阅不同，部分场景 token 非 Verification Token。
			// 这里仅记录告警，不做强拦截，避免因混合订阅（v1+v2）导致合法回调被误拒绝。
			klog.Warningf("feishu callback: non-v2 token mismatch from %s (ignored)", c.ClientIP())
		}
	}

	// Extract action and operator depending on callback format version.
	var action feishuCBAction
	var operatorOpenID string
	var operatorUserID string
	var actionOpenID string
	if cb.Schema == "2.0" {
		action = cb.Event.Action
		operatorOpenID = cb.Event.Operator.OpenID
		operatorUserID = cb.Event.Operator.UserID
		actionOpenID = cb.Event.Action.OpenID
	} else {
		action = cb.Action
		operatorOpenID = cb.Operator.OpenID
		operatorUserID = cb.Operator.UserID
		actionOpenID = cb.Action.OpenID
		if operatorOpenID == "" {
			operatorOpenID = action.OpenID
		}
	}

	// Parse action value
	actionType, _ := action.Value["action"].(string)
	requestIDStr, ok := callbackValueToString(action.Value["request_id"])
	if !ok {
		requestIDStr = ""
	}
	requestID, parseErr := strconv.ParseUint(requestIDStr, 10, 32)
	if parseErr != nil || (actionType != "approve" && actionType != "reject" && actionType != "renew") {
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
			"type":    "error",
			"content": "无效的操作",
		}})
		return
	}

	req, err := model.GetAccessRequest(uint(requestID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
			"type":    "error",
			"content": "申请记录不存在",
		}})
		return
	}

	// Validate status based on action type
	if actionType == "renew" {
		// Renewal only applies to approved (still active) requests
		if req.Status != model.AccessRequestApproved {
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "info",
				"content": fmt.Sprintf("无法续期（当前状态：%s）", localizeStatus(req.Status)),
			}})
			return
		}
	} else {
		// approve / reject only apply to pending requests
		if req.Status != model.AccessRequestPending {
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "info",
				"content": fmt.Sprintf("该申请已处理（状态：%s）", localizeStatus(req.Status)),
			}})
			return
		}
	}

	// For approve/reject: any configured approver can act.
	// For renew: only the designated approver can act.
	if actionType == "renew" {
		if !isApproverMatched(req.ApproverUID, operatorOpenID, operatorUserID, actionOpenID) {
			klog.Warningf(
				"feishu callback: renew approver mismatch req=%d expected=%q operator_open=%q operator_user=%q action_open=%q",
				req.ID,
				req.ApproverUID,
				operatorOpenID,
				operatorUserID,
				actionOpenID,
			)
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "error",
				"content": "您无权操作此申请",
			}})
			return
		}
	} else {
		// approve / reject: check operator is in the configured approver list
		if !isConfiguredApprover(setting, operatorOpenID, operatorUserID, actionOpenID) {
			klog.Warningf(
				"feishu callback: approver not in list req=%d operator_open=%q operator_user=%q action_open=%q",
				req.ID,
				operatorOpenID,
				operatorUserID,
				actionOpenID,
			)
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "error",
				"content": "您无权操作此申请",
			}})
			return
		}
	}

	now := time.Now()
	renewHours := 0
	if actionType == "approve" {
		expiresAt := now.Add(time.Duration(req.DurationHours) * time.Hour)
		req.ExpiresAt = &expiresAt
		req.ApprovedAt = &now
		req.Status = model.AccessRequestApproved

		if err := createTempRole(req); err != nil {
			klog.Errorf("feishu callback: create temp role for request %d: %v", req.ID, err)
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "error",
				"content": "授权失败，请联系管理员",
			}})
			return
		}
	} else if actionType == "renew" {
		// Parse renewal hours from action value
		hoursStr, _ := callbackValueToString(action.Value["hours"])
		var renewErr error
		renewHours, renewErr = strconv.Atoi(hoursStr)
		if renewErr != nil || renewHours <= 0 {
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "error",
				"content": "无效的续期时长",
			}})
			return
		}

		// Extend expiry: add renewal hours to the current expiry time (or now if already past)
		base := now
		if req.ExpiresAt != nil && req.ExpiresAt.After(now) {
			base = *req.ExpiresAt
		}
		newExpiry := base.Add(time.Duration(renewHours) * time.Hour)
		req.ExpiresAt = &newExpiry
		req.ExpiringSoonNotified = false // reset so a new expiring-soon notification can fire

		// Ensure the temp role still exists (it should, since status is approved)
		if req.RoleID == nil {
			// Role was somehow deleted, recreate it
			if err := createTempRole(req); err != nil {
				klog.Errorf("feishu callback: recreate temp role for renewal %d: %v", req.ID, err)
				c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
					"type":    "error",
					"content": "续期失败，请联系管理员",
				}})
				return
			}
		}
	} else {
		req.Status = model.AccessRequestRejected
	}

	if err := model.SaveAccessRequest(req); err != nil {
		klog.Errorf("feishu callback: save request %d: %v", req.ID, err)
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
			"type":    "error",
			"content": "保存失败，请联系管理员",
		}})
		return
	}

	go updateCardToResult(req)

	var toastMsg string
	if actionType == "approve" {
		toastMsg = fmt.Sprintf("已批准 %s 访问命名空间 %s（%s后过期）",
			req.RequesterName, req.Namespace, feishu.FormatDuration(req.DurationHours))
	} else if actionType == "renew" {
		toastMsg = fmt.Sprintf("已为 %s 续期 %d 小时（新到期时间：%s）",
			req.RequesterName, renewHours, req.ExpiresAt.Format("01-02 15:04"))
	} else {
		toastMsg = fmt.Sprintf("已拒绝 %s 的申请", req.RequesterName)
	}
	c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
		"type":    "success",
		"content": toastMsg,
	}})
}

func localizeStatus(s string) string {
	switch s {
	case model.AccessRequestApproved:
		return "已批准"
	case model.AccessRequestRejected:
		return "已拒绝"
	case model.AccessRequestWithdrawn:
		return "已撤回"
	case model.AccessRequestExpired:
		return "已过期"
	default:
		return s
	}
}
