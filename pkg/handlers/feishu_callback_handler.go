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

// feishuCardCallback is the structure Feishu sends on card action.
type feishuCardCallback struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"` // URL verification challenge
	Token     string `json:"token"`
	Action    struct {
		Tag    string                 `json:"tag"`
		Value  map[string]interface{} `json:"value"`
		OpenID string                 `json:"open_id"`
	} `json:"action"`
	Operator struct {
		OpenID string `json:"open_id"`
		UserID string `json:"user_id"`
	} `json:"operator"`
}

// HandleFeishuCardCallback handles POST /api/feishu/card-callback.
// This is a public endpoint called directly by Feishu servers.
func HandleFeishuCardCallback(c *gin.Context) {
	// Read body before signature verification
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

	// Handle URL verification challenge FIRST (Feishu sends this when registering the callback URL).
	// No signature check needed for this handshake.
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

	// Verify signature when verification token is configured
	if token := string(setting.VerificationToken); token != "" {
		timestamp := c.GetHeader("X-Lark-Request-Timestamp")
		nonce := c.GetHeader("X-Lark-Request-Nonce")
		signature := c.GetHeader("X-Lark-Signature")
		if !feishu.VerifyCardSignature(timestamp, nonce, token, string(bodyBytes), signature) {
			klog.Warningf("feishu callback: invalid signature from %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	// Determine operator's open_id
	operatorOpenID := cb.Operator.OpenID
	if operatorOpenID == "" {
		operatorOpenID = cb.Action.OpenID
	}

	// Parse action value
	actionType, _ := cb.Action.Value["action"].(string)
	requestIDStr, _ := cb.Action.Value["request_id"].(string)
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
