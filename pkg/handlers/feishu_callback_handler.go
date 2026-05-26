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
		var bodyToken string
		if cb.Schema == "2.0" {
			bodyToken = cb.Header.Token
		} else {
			bodyToken = cb.Token
		}
		if bodyToken != storedToken {
			klog.Warningf("feishu callback: token mismatch from %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
	}

	// Extract action and operator depending on callback format version.
	var action feishuCBAction
	var operatorOpenID string
	if cb.Schema == "2.0" {
		action = cb.Event.Action
		operatorOpenID = cb.Event.Operator.OpenID
	} else {
		action = cb.Action
		operatorOpenID = cb.Operator.OpenID
		if operatorOpenID == "" {
			operatorOpenID = action.OpenID
		}
	}

	// Parse action value
	actionType, _ := action.Value["action"].(string)
	requestIDStr, _ := action.Value["request_id"].(string)
	requestID, parseErr := strconv.ParseUint(requestIDStr, 10, 32)
	if parseErr != nil || (actionType != "approve" && actionType != "reject") {
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

	if req.Status != model.AccessRequestPending {
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
			"type":    "info",
			"content": fmt.Sprintf("该申请已处理（状态：%s）", localizeStatus(req.Status)),
		}})
		return
	}

	// Only the designated approver may act
	if req.ApproverUID != operatorOpenID {
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
			"type":    "error",
			"content": "您无权操作此申请",
		}})
		return
	}

	now := time.Now()
	if actionType == "approve" {
		expiresAt := now.Add(time.Duration(req.DurationHours) * time.Hour)
		req.ExpiresAt = &expiresAt
		req.Status = model.AccessRequestApproved

		if err := createTempRole(req); err != nil {
			klog.Errorf("feishu callback: create temp role for request %d: %v", req.ID, err)
			c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{
				"type":    "error",
				"content": "授权失败，请联系管理员",
			}})
			return
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
