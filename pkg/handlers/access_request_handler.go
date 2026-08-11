package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/feishu"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
)

const (
	accessSummaryBatchSize   = 20
	accessSummaryConcurrency = 3
	accessSummaryClaimTTL    = 5 * time.Minute
	accessSummaryMaxRetry    = time.Hour
)

var (
	accessSummaryWakeup = make(chan struct{}, 1)
	accessSummarySlots  = make(chan struct{}, accessSummaryConcurrency)
)

// ─── Request/Response types ──────────────────────────────────────────────────

type createAccessRequestBody struct {
	Cluster         string   `json:"cluster" binding:"required"`
	Namespaces      []string `json:"namespaces"`
	RequestType     string   `json:"requestType" binding:"required,oneof=full_update canary_update route_adjust"`
	ReportLink      string   `json:"reportLink"`
	TargetResources []string `json:"targetResources"`
	DurationHours   int      `json:"durationHours" binding:"required,min=1,max=720"`
	RiskLevel       string   `json:"riskLevel" binding:"required,oneof=low medium high"`
	Reason          string   `json:"reason" binding:"required"`
	ApproverUID     string   `json:"approverUid" binding:"required"`
	ApproverName    string   `json:"approverName"`
}

func normalizeReportLink(raw string) (string, error) {
	link := strings.TrimSpace(raw)
	if link == "" {
		return "", fmt.Errorf("测试报告链接不能为空")
	}
	if len(link) > 500 {
		return "", fmt.Errorf("测试报告链接不能超过 500 个字符")
	}
	parsed, err := url.ParseRequestURI(link)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("测试报告链接必须是有效的 HTTP(S) URL")
	}
	return link, nil
}

func normalizeTargetResources(resources []string) ([]string, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("请至少选择一个网关配置 (configmap)")
	}
	if len(resources) > 100 {
		return nil, fmt.Errorf("一次最多选择 100 个网关配置 (configmap)")
	}

	seen := make(map[string]struct{}, len(resources))
	result := make([]string, 0, len(resources))
	for _, raw := range resources {
		name := strings.TrimSpace(raw)
		if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
			return nil, fmt.Errorf("无效的 ConfigMap 名称 %q: %s", raw, strings.Join(errs, "; "))
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result, nil
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
	card := feishu.BuildRequestCardFromData(feishu.RequestCardData{
		RequestID: req.ID, RequesterName: requesterName, Cluster: req.Cluster,
		Namespace: req.Namespace, RequestType: req.RequestType, ReportLink: req.ReportLink,
		TargetResources: req.TargetResources, DurationHours: req.DurationHours,
		RiskLevel: req.RiskLevel, Reason: req.Reason,
		ApproverOpenID: req.ApproverUID, ApproverName: req.ApproverName,
	})
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

// buildTempRole builds a least-privilege role from an access request. Unknown or
// incomplete request data must fail closed instead of falling back to wildcard
// permissions.
func buildTempRole(req *model.AccessRequest) (*model.Role, error) {
	if req == nil {
		return nil, fmt.Errorf("access request is nil")
	}
	if req.ExpiresAt == nil {
		return nil, fmt.Errorf("access request %d has no expiry", req.ID)
	}
	clusterName := strings.TrimSpace(req.Cluster)
	if clusterName == "" {
		return nil, fmt.Errorf("access request %d has no cluster", req.ID)
	}

	roleName := fmt.Sprintf("temp-access-req-%d", req.ID)
	namespaceNames := make([]string, 0)
	for _, namespace := range strings.Split(req.Namespace, ",") {
		if namespace = strings.TrimSpace(namespace); namespace != "" {
			namespaceNames = append(namespaceNames, namespace)
		}
	}
	if len(namespaceNames) == 0 {
		return nil, fmt.Errorf("access request %d has no namespace", req.ID)
	}

	role := &model.Role{
		Name:        roleName,
		Description: fmt.Sprintf("临时授权 #%d: 用户 %s 访问 %s/%s (过期: %s)", req.ID, req.RequesterName, clusterName, req.Namespace, req.ExpiresAt.Format("2006-01-02 15:04")),
		Clusters:    []string{clusterName},
		Namespaces:  namespaceNames,
	}

	switch req.RequestType {
	case model.RequestTypeRouteAdjust:
		if len(namespaceNames) != 1 || namespaceNames[0] != "envoy-gateway-system" {
			return nil, fmt.Errorf("route adjustment request %d has invalid namespace %q", req.ID, req.Namespace)
		}
		targetResources, err := normalizeTargetResources([]string(req.TargetResources))
		if err != nil {
			return nil, fmt.Errorf("route adjustment request %d: %w", req.ID, err)
		}
		// RouteAdjust: only configmaps with resourceNames, only create/update/patch (no delete)
		role.Resources = []string{"configmaps"}
		role.ResourceNames = model.SliceString(targetResources)
		role.Verbs = []string{"get", "list", "create", "update", "patch"}
	case model.RequestTypeFullUpdate, model.RequestTypeCanaryUpdate:
		// FullUpdate / CanaryUpdate retain full access within the requested namespace.
		role.Resources = []string{"*"}
		role.Verbs = []string{"*"}
	default:
		return nil, fmt.Errorf("access request %d has unsupported request type %q", req.ID, req.RequestType)
	}
	return role, nil
}

// createTempRole creates and assigns a temporary Kite role scoped to the request type.
func createTempRole(req *model.AccessRequest) error {
	role, err := buildTempRole(req)
	if err != nil {
		return err
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
	if err := model.AddRoleAssignment(role.Name, model.SubjectTypeUser, user.Username); err != nil {
		_ = model.DB.Delete(role).Error
		return fmt.Errorf("assign role: %w", err)
	}
	rbac.TriggerSync()
	return nil
}

// reconcileActiveRouteAdjustmentRoles repairs roles created by older versions
// that granted wildcard permissions for route-adjustment requests. It only
// touches active roles that are linked to an approved route-adjustment request.
func reconcileActiveRouteAdjustmentRoles() {
	var requests []model.AccessRequest
	if err := model.DB.Where(
		"status = ? AND request_type = ? AND role_id IS NOT NULL",
		model.AccessRequestApproved,
		model.RequestTypeRouteAdjust,
	).Find(&requests).Error; err != nil {
		klog.Warningf("access_request: failed to find route adjustment roles to reconcile: %v", err)
		return
	}

	repaired := 0
	for i := range requests {
		req := &requests[i]
		desired, err := buildTempRole(req)
		if err != nil {
			klog.Warningf("access_request: refusing to reconcile invalid request %d: %v", req.ID, err)
			continue
		}
		result := model.DB.Model(&model.Role{}).
			Where("id = ?", *req.RoleID).
			Select("Clusters", "Namespaces", "Resources", "ResourceNames", "Verbs").
			Updates(desired)
		if result.Error != nil {
			klog.Warningf("access_request: failed to reconcile role %d for request %d: %v", *req.RoleID, req.ID, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			repaired++
		}
	}
	if repaired > 0 {
		klog.Infof("access_request: reconciled %d route adjustment role(s) to selected ConfigMaps", repaired)
		rbac.TriggerSync()
	}
}

// deleteTempRole removes the temporary role (and cascades to RoleAssignment).
// Callers must not mark access as expired when this fails: doing so would make
// the UI claim that permission was revoked while the role may still be active.
func deleteTempRole(roleID uint) error {
	if err := model.DB.Delete(&model.Role{}, roleID).Error; err != nil {
		return fmt.Errorf("delete temp role %d: %w", roleID, err)
	}
	rbac.TriggerSync()
	return nil
}

// recordAccessRequestAudit logs an access request action (approve/reject/revoke/renew) to the
// ResourceHistory table so it shows up in the Audit page.
func recordAccessRequestAudit(c *gin.Context, req *model.AccessRequest, action string) {
	currentUser, ok := c.MustGet("user").(model.User)
	if !ok {
		return
	}
	recordAccessRequestAuditByID(req, action, currentUser.ID, "manual")
}

// recordAccessRequestAuditByID logs an access request action with explicit operator ID and source.
// Used by feishu callback handler which has no Gin user context.
func recordAccessRequestAuditByID(req *model.AccessRequest, action string, operatorID uint, source string) {
	history := model.ResourceHistory{
		ClusterName:     req.Cluster,
		ResourceType:    "AccessRequest",
		ResourceName:    fmt.Sprintf("#%d-%s", req.ID, req.RequesterName),
		Namespace:       req.Namespace,
		OperationType:   action,
		OperationSource: source,
		Success:         true,
		OperatorID:      operatorID,
	}
	if err := model.DB.Create(&history).Error; err != nil {
		klog.Warningf("access_request: failed to record audit for request %d action %s: %v", req.ID, action, err)
	}
}

func buildAccessRequestResultCard(req *model.AccessRequest) map[string]interface{} {
	requesterName := req.RequesterName
	if requesterName == "" {
		requesterName = fmt.Sprintf("用户#%d", req.RequesterID)
	}
	return feishu.BuildResultCardFromData(feishu.RequestCardData{
		RequestID: req.ID, RequesterName: requesterName, Cluster: req.Cluster,
		Namespace: req.Namespace, RequestType: req.RequestType, ReportLink: req.ReportLink,
		TargetResources: req.TargetResources, DurationHours: req.DurationHours,
		RiskLevel: req.RiskLevel, Reason: req.Reason, ApproverName: req.ApproverName,
	}, req.Status, req.ReviewNote)
}

func updateCardToResult(req *model.AccessRequest) {
	bot, setting, err := getFeishuBot()
	if err != nil || bot == nil || req.MessageID == "" || setting.GroupChatID == "" {
		return
	}
	card := buildAccessRequestResultCard(req)
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
	selectedClusterValue, exists := c.Get("cluster")
	selectedCluster, ok := selectedClusterValue.(*cluster.ClientSet)
	if !exists || !ok || selectedCluster == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申请集群上下文缺失"})
		return
	}
	if body.Cluster != selectedCluster.Name {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申请集群与请求集群不一致"})
		return
	}

	// Type-specific validation
	switch body.RequestType {
	case model.RequestTypeFullUpdate:
		reportLink, err := normalizeReportLink(body.ReportLink)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.ReportLink = reportLink
		body.TargetResources = nil
		if len(body.Namespaces) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one namespace is required"})
			return
		}
	case model.RequestTypeCanaryUpdate:
		reportLink, err := normalizeReportLink(body.ReportLink)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.ReportLink = reportLink
		body.TargetResources = nil
		if len(body.Namespaces) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one namespace is required"})
			return
		}
	case model.RequestTypeRouteAdjust:
		targetResources, err := normalizeTargetResources(body.TargetResources)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.TargetResources = targetResources
		body.ReportLink = ""
		// Force namespace to envoy-gateway-system regardless of user selection
		body.Namespaces = []string{"envoy-gateway-system"}
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
		RequesterID:     currentUser.ID,
		RequesterName:   requesterName,
		Cluster:         body.Cluster,
		Namespace:       strings.Join(body.Namespaces, ","),
		RequestType:     body.RequestType,
		ReportLink:      body.ReportLink,
		TargetResources: model.SliceString(body.TargetResources),
		DurationHours:   body.DurationHours,
		RiskLevel:       body.RiskLevel,
		Reason:          body.Reason,
		ApproverUID:     body.ApproverUID,
		ApproverName:    resolvedApproverName,
		Status:          model.AccessRequestPending,
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
	card := feishu.BuildReminderCardFromData(feishu.RequestCardData{
		RequestID: req.ID, RequesterName: requesterName, Cluster: req.Cluster,
		Namespace: req.Namespace, RequestType: req.RequestType, ReportLink: req.ReportLink,
		TargetResources: req.TargetResources, DurationHours: req.DurationHours,
		RiskLevel: req.RiskLevel, Reason: req.Reason,
		ApproverOpenID: req.ApproverUID, ApproverName: req.ApproverName,
	})
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
	page := 1
	size := 20
	if rawPage := strings.TrimSpace(c.Query("page")); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page parameter"})
			return
		}
		page = parsed
	}
	if rawSize := strings.TrimSpace(c.Query("size")); rawSize != "" {
		parsed, err := strconv.Atoi(rawSize)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid size parameter"})
			return
		}
		size = parsed
	}

	reqs, total, err := model.ListAccessRequests(page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list requests"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"requests": reqs,
		"total":    total,
		"page":     page,
		"size":     size,
	})
}

// ApproveAccess handles PUT /api/v1/admin/access-requests/:id/approve
func ApproveAccess(c *gin.Context) {
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
	if req.Status != model.AccessRequestPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only pending requests can be approved"})
		return
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(req.DurationHours) * time.Hour)
	req.ExpiresAt = &expiresAt
	req.ApprovedAt = &now
	req.Status = model.AccessRequestApproved

	if err := createTempRole(req); err != nil {
		klog.Errorf("access_request: approve failed to create temp role for request %d: %v", req.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp role"})
		return
	}

	if err := model.SaveAccessRequest(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save"})
		return
	}

	// Record audit + update Feishu card
	recordAccessRequestAudit(c, req, "approve")
	go updateCardToResult(req)

	c.JSON(http.StatusOK, req)
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
		if err := deleteTempRole(*req.RoleID); err != nil {
			klog.Errorf("access_request: revoke failed for request %d: %v", req.ID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke temporary role"})
			return
		}
	}

	endedAt := time.Now()
	expired, err := model.ExpireAccessRequestAndQueueSummary(req.ID, endedAt, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save"})
		return
	}
	if !expired {
		c.JSON(http.StatusConflict, gin.H{"error": "request was already processed"})
		return
	}
	req.Status = model.AccessRequestExpired
	req.RoleID = nil
	req.EndedAt = &endedAt
	req.SummaryStatus = model.AccessSummaryPending

	// Update the Feishu card to show expired status
	go updateCardToResult(req)

	// Record audit and wake the durable summary worker.
	recordAccessRequestAudit(c, req, "revoke")
	triggerAccessSummaryWorker()

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
	if setting.Enabled && setting.AppID != "" && setting.GroupChatID != "" {
		if err := model.RetryFailedAccessSummariesNow(); err != nil {
			klog.Warningf("access_request summary: failed to wake retry jobs after Feishu settings update: %v", err)
		} else {
			triggerAccessSummaryWorker()
		}
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

		// Run immediately so restart recovery does not wait for the first tick.
		reconcileActiveRouteAdjustmentRoles()
		runAccessRequestMaintenanceSafely()
		for {
			select {
			case <-ticker.C:
				runAccessRequestMaintenanceSafely()
			case <-accessSummaryWakeup:
				processAccessSummaryQueueSafely()
			}
		}
	}()
}

func runAccessRequestMaintenanceSafely() {
	defer func() {
		if recovered := recover(); recovered != nil {
			klog.Errorf("access_request maintenance worker panic: %v", recovered)
		}
	}()
	runAccessRequestMaintenance()
}

func runAccessRequestMaintenance() {
	notifyExpiringSoonRequests()
	expireAccessRequests()
	processAccessSummaryQueue()
}

func triggerAccessSummaryWorker() {
	select {
	case accessSummaryWakeup <- struct{}{}:
	default:
	}
}

func newAccessSummaryClaimToken() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func accessSummaryRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Minute
	for i := 1; i < attempt && delay < accessSummaryMaxRetry; i++ {
		delay *= 2
	}
	if delay > accessSummaryMaxRetry {
		return accessSummaryMaxRetry
	}
	return delay
}

func processAccessSummaryQueue() {
	now := time.Now()
	reqs, err := model.ListAccessSummaryCandidates(now, now.Add(-accessSummaryClaimTTL), accessSummaryBatchSize)
	if err != nil {
		klog.Errorf("access_request summary worker: list jobs: %v", err)
		return
	}

	for i := range reqs {
		select {
		case accessSummarySlots <- struct{}{}:
		default:
			return
		}

		req := reqs[i]
		claimToken := newAccessSummaryClaimToken()
		claimed, claimErr := model.ClaimAccessSummary(req.ID, claimToken, now, now.Add(-accessSummaryClaimTTL))
		if claimErr != nil {
			<-accessSummarySlots
			klog.Errorf("access_request summary worker: claim request #%d: %v", req.ID, claimErr)
			continue
		}
		if !claimed {
			<-accessSummarySlots
			continue
		}

		req.SummaryAttempts++
		req.SummaryClaimToken = claimToken
		go func(req model.AccessRequest) {
			defer func() {
				if recovered := recover(); recovered != nil {
					nextRetryAt := time.Now().Add(accessSummaryRetryDelay(req.SummaryAttempts))
					_, _ = model.MarkAccessSummaryFailed(req.ID, req.SummaryClaimToken, fmt.Sprintf("worker panic: %v", recovered), nextRetryAt)
					klog.Errorf("access_request summary worker panic for request #%d: %v", req.ID, recovered)
				}
				<-accessSummarySlots
				triggerAccessSummaryWorker()
			}()

			result, summaryErr := generateAndSendAccessUsageSummary(&req)
			if summaryErr != nil {
				nextRetryAt := time.Now().Add(accessSummaryRetryDelay(req.SummaryAttempts))
				var aiErr *accessSummaryAIError
				var deliveryErr *accessSummaryDeliveryError
				var notificationErr *accessSummaryNotificationError
				var updated bool
				var markErr error
				switch {
				case errors.As(summaryErr, &aiErr):
					updated, markErr = model.MarkAccessSummaryAIFailed(
						req.ID, req.SummaryClaimToken, aiErr.reason, nextRetryAt,
					)
				case errors.As(summaryErr, &deliveryErr):
					updated, markErr = model.MarkAccessSummaryDeliveryFailed(
						req.ID, req.SummaryClaimToken, conciseSummaryFailureReason(deliveryErr), nextRetryAt,
					)
				case errors.As(summaryErr, &notificationErr):
					updated, markErr = model.MarkAccessSummaryNotificationFailed(
						req.ID, req.SummaryClaimToken, notificationErr.reason, notificationErr.aiAttempts, nextRetryAt,
					)
				default:
					updated, markErr = model.MarkAccessSummaryFailed(
						req.ID, req.SummaryClaimToken, conciseSummaryFailureReason(summaryErr), nextRetryAt,
					)
				}
				if markErr != nil {
					klog.Errorf("access_request summary worker: persist failure for request #%d: %v", req.ID, markErr)
				} else if updated {
					klog.Warningf("access_request summary worker: request #%d failed (attempt %d), retry at %s: %v",
						req.ID, req.SummaryAttempts, nextRetryAt.Format(time.RFC3339), summaryErr)
				}
				return
			}

			var updated bool
			var markErr error
			if result.FailedNotified {
				updated, markErr = model.MarkAccessSummaryFailedNotified(
					req.ID, req.SummaryClaimToken, result.MessageID, result.FailureReason, result.AIAttempts, time.Now(),
				)
			} else {
				updated, markErr = model.MarkAccessSummarySent(req.ID, req.SummaryClaimToken, result.MessageID, time.Now())
			}
			if markErr != nil {
				klog.Errorf("access_request summary worker: complete request #%d: %v", req.ID, markErr)
			} else if updated {
				if result.FailedNotified {
					klog.Warningf("access_request summary worker: notified terminal failure for request #%d: %s", req.ID, result.FailureReason)
				} else {
					klog.Infof("access_request summary worker: sent summary for request #%d (attempt %d)", req.ID, req.SummaryAttempts)
				}
			}
		}(req)
	}
}

func processAccessSummaryQueueSafely() {
	defer func() {
		if recovered := recover(); recovered != nil {
			klog.Errorf("access_request summary queue panic: %v", recovered)
		}
	}()
	processAccessSummaryQueue()
}

// notifyExpiringSoonRequests sends a Feishu thread reply with renewal buttons
// for approved requests that will expire within 10 minutes.
//
// Race-safety: we first SELECT candidate rows (expiring_soon_notified=false),
// then for each row atomically flip the flag via
// ClaimExpiringSoonNotification (UPDATE … WHERE flag=false). Only the caller
// that flips 0→1 (RowsAffected==1) sends the card. This guarantees each
// request gets exactly one notification even under concurrent ticks / replicas.
func notifyExpiringSoonRequests() {
	reqs, err := model.ListExpiringSoonRequests(10 * time.Minute)
	if err != nil {
		klog.Errorf("access_request expiring-soon worker: %v", err)
		return
	}
	for i := range reqs {
		req := &reqs[i]

		// Atomically claim the notification slot. If another tick/replica
		// already claimed it, skip — this is what prevents duplicates.
		claimed, err := model.ClaimExpiringSoonNotification(req.ID)
		if err != nil {
			klog.Errorf("access_request expiring-soon: claim failed for request %d: %v", req.ID, err)
			continue
		}
		if !claimed {
			continue // already claimed by another tick/replica
		}

		bot, setting, err := getFeishuBot()
		if err != nil || bot == nil || req.MessageID == "" || setting.GroupChatID == "" {
			// Already claimed (flag is now true), so we won't retry. Log it.
			klog.Infof("access_request expiring-soon: claimed #%d but Feishu not configured, skipping", req.ID)
			continue
		}

		expiresAtStr := "-"
		if req.ExpiresAt != nil {
			expiresAtStr = req.ExpiresAt.Format("01-02 15:04")
		}
		card := feishu.BuildExpiringSoonCard(req.ID, req.RequesterName, req.Cluster, req.Namespace, expiresAtStr)

		if err := bot.ReplyCard(req.MessageID, card); err != nil {
			klog.Warningf("access_request expiring-soon: failed to reply card for request %d: %v", req.ID, err)
			// Release the claim so a later tick can retry sending.
			req.ExpiringSoonNotified = false
			_ = model.SaveAccessRequest(req)
			continue
		}

		klog.Infof("access_request: sent expiring-soon notification for request #%d (%s → %s)", req.ID, req.RequesterName, req.Namespace)
	}
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
			if err := deleteTempRole(*req.RoleID); err != nil {
				klog.Errorf("access_request expiry: revoke role for request %d: %v", req.ID, err)
				continue
			}
		}

		endedAt := time.Now()
		expired, expireErr := model.ExpireAccessRequestAndQueueSummary(req.ID, endedAt, true)
		if expireErr != nil {
			klog.Errorf("access_request expiry: save %d: %v", req.ID, expireErr)
			continue
		}
		if !expired {
			continue
		}
		req.Status = model.AccessRequestExpired
		req.RoleID = nil
		req.EndedAt = &endedAt
		req.SummaryStatus = model.AccessSummaryPending
		klog.Infof("access_request: expired request #%d (%s → %s)", req.ID, req.RequesterName, req.Namespace)

		// Update the Feishu card to show expired status
		go updateCardToResult(req)

	}
}
