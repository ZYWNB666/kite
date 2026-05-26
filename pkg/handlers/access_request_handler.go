package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/feishu"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/klog/v2"
)

// ─── Request/Response types ──────────────────────────────────────────────────

type createAccessRequestBody struct {
	Namespaces    []string `json:"namespaces"`
	DurationHours int      `json:"durationHours" binding:"required,min=1,max=720"`
	Reason        string   `json:"reason" binding:"required"`
	ApproverUID   string   `json:"approverUid" binding:"required"`
	ApproverName  string   `json:"approverName"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func getFeishuBot() (*feishu.BotClient, *model.FeishuNotificationSetting, error) {
	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		return nil, nil, err
	}
	if !setting.Enabled || setting.AppID == "" {
		return nil, setting, nil
	}
	return feishu.NewBotClient(setting.AppID, string(setting.AppSecret)), setting, nil
}

func sendRequestCard(req *model.AccessRequest) {
	bot, setting, err := getFeishuBot()
	if err != nil || bot == nil || setting.GroupChatID == "" {
		return
	}
	requesterName := req.RequesterName
	if requesterName == "" {
		requesterName = fmt.Sprintf("用户#%d", req.RequesterID)
	}
	card := feishu.BuildRequestCard(req.ID, requesterName, req.Namespace, req.DurationHours, req.Reason, req.ApproverUID, req.ApproverName)
	msgID, err := bot.SendCard(setting.GroupChatID, card)
	if err != nil {
		klog.Warningf("access_request: failed to send feishu card for request %d: %v", req.ID, err)
		return
	}
	req.MessageID = msgID
	if saveErr := model.SaveAccessRequest(req); saveErr != nil {
		klog.Warningf("access_request: failed to save message_id for request %d: %v", req.ID, saveErr)
	}
}

// createTempRole creates a temporary Kite role granting full access to the requested namespace.
func createTempRole(req *model.AccessRequest) error {
	roleName := fmt.Sprintf("temp-access-req-%d", req.ID)
	namespaces := strings.Split(req.Namespace, ",")
	role := &model.Role{
		Name:        roleName,
		Description: fmt.Sprintf("临时授权 #%d: 用户 %s 访问 %s (过期: %s)", req.ID, req.RequesterName, req.Namespace, req.ExpiresAt.Format("2006-01-02 15:04")),
		Clusters:    []string{"*"},
		Resources:   []string{"*"},
		Namespaces:  namespaces,
		Verbs:       []string{"*"},
	}
	if err := model.DB.Create(role).Error; err != nil {
		return fmt.Errorf("create temp role: %w", err)
	}
	req.RoleID = &role.ID

	// Find user by ID and assign role by username
	user, err := model.GetUserByID(uint64(req.RequesterID))
	if err != nil {
		// cleanup role
		_ = model.DB.Delete(role).Error
		return fmt.Errorf("find requester: %w", err)
	}
	if err := model.AddRoleAssignment(roleName, model.SubjectTypeUser, user.Username); err != nil {
		_ = model.DB.Delete(role).Error
		return fmt.Errorf("assign role: %w", err)
	}
	rbac.TriggerSync()
	return nil
}

// deleteTempRole removes the temporary role (and cascades to RoleAssignment).
func deleteTempRole(roleID uint) {
	if err := model.DB.Delete(&model.Role{}, roleID).Error; err != nil {
		klog.Warningf("access_request: failed to delete temp role %d: %v", roleID, err)
		return
	}
	rbac.TriggerSync()
}

func updateCardToResult(req *model.AccessRequest) {
	bot, setting, err := getFeishuBot()
	if err != nil || bot == nil || req.MessageID == "" || setting.GroupChatID == "" {
		return
	}
	requesterName := req.RequesterName
	if requesterName == "" {
		requesterName = fmt.Sprintf("用户#%d", req.RequesterID)
	}
	card := feishu.BuildResultCard(req.ID, requesterName, req.Namespace, req.DurationHours, req.Reason, req.ApproverName, req.Status, req.ReviewNote)
	if err := bot.PatchCard(req.MessageID, card); err != nil {
		klog.Warningf("access_request: failed to update card for request %d: %v", req.ID, err)
	}
}

// ─── User Handlers ───────────────────────────────────────────────────────────

// CreateAccessRequest handles POST /api/v1/access-requests
func CreateAccessRequest(c *gin.Context) {
	currentUser, ok := c.MustGet("user").(model.User)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var body createAccessRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(body.Namespaces) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one namespace is required"})
		return
	}

	// Validate approver is in the configured list
	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}
	approvers := setting.GetApprovers()
	approverValid := false
	resolvedApproverName := body.ApproverName
	for _, a := range approvers {
		if a.OpenID == body.ApproverUID {
			approverValid = true
			resolvedApproverName = a.Name
			break
		}
	}
	if len(approvers) > 0 && !approverValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approver"})
		return
	}

	requesterName := currentUser.Name
	if requesterName == "" {
		requesterName = currentUser.Username
	}

	req := &model.AccessRequest{
		RequesterID:   currentUser.ID,
		RequesterName: requesterName,
		Namespace:     strings.Join(body.Namespaces, ","),
		DurationHours: body.DurationHours,
		Reason:        body.Reason,
		ApproverUID:   body.ApproverUID,
		ApproverName:  resolvedApproverName,
		Status:        model.AccessRequestPending,
	}
	if err := model.CreateAccessRequest(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Send Feishu notification asynchronously
	go sendRequestCard(req)

	c.JSON(http.StatusCreated, req)
}

// ListMyAccessRequests handles GET /api/v1/access-requests
func ListMyAccessRequests(c *gin.Context) {
	currentUser, ok := c.MustGet("user").(model.User)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	reqs, err := model.ListMyAccessRequests(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list requests"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": reqs})
}

// WithdrawAccessRequest handles PUT /api/v1/access-requests/:id/withdraw
func WithdrawAccessRequest(c *gin.Context) {
	currentUser, ok := c.MustGet("user").(model.User)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	req, err := model.GetAccessRequest(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	if req.RequesterID != currentUser.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your request"})
		return
	}
	if req.Status != model.AccessRequestPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only pending requests can be withdrawn"})
		return
	}
	req.Status = model.AccessRequestWithdrawn
	if err := model.SaveAccessRequest(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to withdraw"})
		return
	}
	go updateCardToResult(req)
	c.JSON(http.StatusOK, req)
}

// RemindAccessRequest handles POST /api/v1/access-requests/:id/remind
func RemindAccessRequest(c *gin.Context) {
	currentUser, ok := c.MustGet("user").(model.User)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	req, err := model.GetAccessRequest(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	if req.RequesterID != currentUser.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your request"})
		return
	}
	if req.Status != model.AccessRequestPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "can only remind on pending requests"})
		return
	}

	bot, setting, err := getFeishuBot()
	if err != nil || bot == nil || setting.GroupChatID == "" {
		c.JSON(http.StatusOK, gin.H{"message": "reminder sent (feishu not configured)"})
		return
	}
	requesterName := req.RequesterName
	if requesterName == "" {
		requesterName = fmt.Sprintf("用户#%d", req.RequesterID)
	}
	card := feishu.BuildReminderCard(req.ID, requesterName, req.Namespace, req.DurationHours, req.Reason, req.ApproverUID, req.ApproverName)
	msgID, err := bot.SendCard(setting.GroupChatID, card)
	if err != nil {
		klog.Warningf("access_request: failed to send reminder for request %d: %v", req.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send reminder"})
		return
	}
	// Update stored message_id to latest reminder message
	req.MessageID = msgID
	_ = model.SaveAccessRequest(req)
	c.JSON(http.StatusOK, gin.H{"message": "reminder sent"})
}

// ─── Admin Handlers ───────────────────────────────────────────────────────────

// ListAllAccessRequests handles GET /api/v1/admin/access-requests
func ListAllAccessRequests(c *gin.Context) {
	reqs, err := model.ListAccessRequests()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list requests"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": reqs})
}

// RevokeAccess handles PUT /api/v1/admin/access-requests/:id/revoke
func RevokeAccess(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	req, err := model.GetAccessRequest(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	if req.Status != model.AccessRequestApproved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "can only revoke approved permissions"})
		return
	}
	if req.RoleID != nil {
		deleteTempRole(*req.RoleID)
	}
	req.Status = model.AccessRequestExpired
	req.RoleID = nil
	if err := model.SaveAccessRequest(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "access revoked"})
}

// ─── Feishu Settings Handlers ─────────────────────────────────────────────────

type feishuSettingUpdateBody struct {
	AppID             string                 `json:"appId"`
	AppSecret         string                 `json:"appSecret"` // empty = keep existing
	GroupChatID       string                 `json:"groupChatId"`
	VerificationToken string                 `json:"verificationToken"` // empty = keep existing
	Approvers         []model.FeishuApprover `json:"approvers"`
	Enabled           bool                   `json:"enabled"`
}

type feishuSettingResponse struct {
	ID          uint                   `json:"id"`
	AppID       string                 `json:"appId"`
	GroupChatID string                 `json:"groupChatId"`
	Approvers   []model.FeishuApprover `json:"approvers"`
	Enabled     bool                   `json:"enabled"`
	// Secret fields masked
	AppSecretSet         bool `json:"appSecretSet"`
	VerificationTokenSet bool `json:"verificationTokenSet"`
}

// GetFeishuSetting handles GET /api/v1/admin/feishu-setting
func GetFeishuSetting(c *gin.Context) {
	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load setting"})
		return
	}
	c.JSON(http.StatusOK, feishuSettingResponse{
		ID:                   setting.ID,
		AppID:                setting.AppID,
		GroupChatID:          setting.GroupChatID,
		Approvers:            setting.GetApprovers(),
		Enabled:              setting.Enabled,
		AppSecretSet:         string(setting.AppSecret) != "",
		VerificationTokenSet: string(setting.VerificationToken) != "",
	})
}

// UpdateFeishuSetting handles PUT /api/v1/admin/feishu-setting
func UpdateFeishuSetting(c *gin.Context) {
	var body feishuSettingUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load setting"})
		return
	}
	setting.AppID = body.AppID
	setting.GroupChatID = body.GroupChatID
	setting.Enabled = body.Enabled
	if body.AppSecret != "" {
		setting.AppSecret = model.SecretString(body.AppSecret)
	}
	if body.VerificationToken != "" {
		setting.VerificationToken = model.SecretString(body.VerificationToken)
	}
	setting.SetApprovers(body.Approvers)
	if err := model.SaveFeishuNotificationSetting(setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save setting"})
		return
	}
	c.JSON(http.StatusOK, feishuSettingResponse{
		ID:                   setting.ID,
		AppID:                setting.AppID,
		GroupChatID:          setting.GroupChatID,
		Approvers:            setting.GetApprovers(),
		Enabled:              setting.Enabled,
		AppSecretSet:         string(setting.AppSecret) != "",
		VerificationTokenSet: string(setting.VerificationToken) != "",
	})
}

// GetFeishuApprovers handles GET /api/v1/feishu-approvers  (used by request dialog)
func GetFeishuApprovers(c *gin.Context) {
	setting, err := model.GetFeishuNotificationSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load setting"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approvers": setting.GetApprovers()})
}

// ─── Background Expiry Worker ─────────────────────────────────────────────────

// StartAccessRequestExpiryWorker runs a goroutine that periodically expires approved
// access requests whose ExpiresAt has passed.
func StartAccessRequestExpiryWorker() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			expireAccessRequests()
		}
	}()
}

func expireAccessRequests() {
	reqs, err := model.ListExpiredActiveRequests()
	if err != nil {
		klog.Errorf("access_request expiry worker: %v", err)
		return
	}
	for i := range reqs {
		req := &reqs[i]
		if req.RoleID != nil {
			deleteTempRole(*req.RoleID)
		}
		req.Status = model.AccessRequestExpired
		req.RoleID = nil
		if err := model.SaveAccessRequest(req); err != nil {
			klog.Errorf("access_request expiry: save %d: %v", req.ID, err)
		}
		klog.Infof("access_request: expired request #%d (%s → %s)", req.ID, req.RequesterName, req.Namespace)
	}
}
