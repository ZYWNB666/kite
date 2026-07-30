package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	AccessRequestPending   = "pending"
	AccessRequestApproved  = "approved"
	AccessRequestRejected  = "rejected"
	AccessRequestWithdrawn = "withdrawn"
	AccessRequestExpired   = "expired"

	AccessSummaryPending        = "pending"
	AccessSummaryProcessing     = "processing"
	AccessSummaryFailed         = "failed"
	AccessSummarySent           = "sent"
	AccessSummaryFailedNotified = "failed_notified"
)

// AccessRequest records a namespace permission request submitted by a user.
type AccessRequest struct {
	Model
	RequesterID             uint       `json:"requesterId" gorm:"index;not null"`
	RequesterName           string     `json:"requesterName" gorm:"type:varchar(100)"`
	Cluster                 string     `json:"cluster" gorm:"type:varchar(255)"`
	Namespace               string     `json:"namespace" gorm:"type:varchar(255);not null"`
	DurationHours           int        `json:"durationHours" gorm:"not null"`
	RiskLevel               string     `json:"riskLevel" gorm:"type:varchar(50);not null;default:'low'"`
	Reason                  string     `json:"reason" gorm:"type:text"`
	ApproverUID             string     `json:"approverUid" gorm:"type:varchar(255)"` // Feishu open_id of approver
	ApproverName            string     `json:"approverName" gorm:"type:varchar(100)"`
	Status                  string     `json:"status" gorm:"type:varchar(20);index;not null;default:'pending'"`
	ExpiresAt               *time.Time `json:"expiresAt"`
	ApprovedAt              *time.Time `json:"approvedAt"`                                                      // when the request was approved (for usage query)
	EndedAt                 *time.Time `json:"endedAt"`                                                         // when access actually ended (scheduled expiry or manual revoke)
	ExpiringSoonNotified    bool       `json:"expiringSoonNotified" gorm:"type:boolean;not null;default:false"` // whether the expiring-soon notification has been sent
	MessageID               string     `json:"messageId" gorm:"type:varchar(255)"`                              // Feishu message_id for card update
	RoleID                  *uint      `json:"roleId"`                                                          // temp role ID, deleted on revoke/expire
	ReviewNote              string     `json:"reviewNote" gorm:"type:text"`
	SummaryStatus           string     `json:"summaryStatus" gorm:"type:varchar(20);index"`
	SummaryAttempts         int        `json:"summaryAttempts" gorm:"not null;default:0"`
	SummaryAIAttempts       int        `json:"summaryAiAttempts" gorm:"not null;default:0"`
	SummaryDeliveryAttempts int        `json:"summaryDeliveryAttempts" gorm:"not null;default:0"`
	SummaryLastError        string     `json:"summaryLastError" gorm:"type:text"`
	SummaryNextRetryAt      *time.Time `json:"summaryNextRetryAt" gorm:"index"`
	SummaryClaimedAt        *time.Time `json:"summaryClaimedAt" gorm:"index"`
	SummaryClaimToken       string     `json:"-" gorm:"type:varchar(64)"`
	SummaryCompletedAt      *time.Time `json:"summaryCompletedAt"`
	SummaryMessageID        string     `json:"summaryMessageId" gorm:"type:varchar(255)"`
	SummaryContent          string     `json:"-" gorm:"type:text"`
	SummaryStats            string     `json:"-" gorm:"type:text"`
}

// FeishuNotificationSetting stores the Feishu bot configuration (singleton, id=1).
type FeishuNotificationSetting struct {
	Model
	AppID             string       `json:"appId" gorm:"type:varchar(255)"`
	AppSecret         SecretString `json:"appSecret" gorm:"type:text"`
	GroupChatID       string       `json:"groupChatId" gorm:"type:varchar(255)"`
	VerificationToken SecretString `json:"verificationToken" gorm:"type:text"`
	ApproversJSON     string       `json:"-" gorm:"column:approvers;type:text"`
	Enabled           bool         `json:"enabled" gorm:"type:boolean;not null;default:false"`
}

// FeishuApprover is one entry in the configurable approver list.
type FeishuApprover struct {
	Name   string `json:"name"`
	OpenID string `json:"openId"` // Feishu open_id
}

// GetApprovers deserialises the stored JSON approver list.
func (s *FeishuNotificationSetting) GetApprovers() []FeishuApprover {
	if s.ApproversJSON == "" {
		return nil
	}
	var approvers []FeishuApprover
	_ = json.Unmarshal([]byte(s.ApproversJSON), &approvers)
	return approvers
}

// SetApprovers serialises the approver list and stores it.
func (s *FeishuNotificationSetting) SetApprovers(approvers []FeishuApprover) {
	b, _ := json.Marshal(approvers)
	s.ApproversJSON = string(b)
}

// GetFeishuNotificationSetting returns the singleton setting (auto-creates if absent).
func GetFeishuNotificationSetting() (*FeishuNotificationSetting, error) {
	var setting FeishuNotificationSetting
	err := DB.First(&setting, 1).Error
	if err != nil {
		// Auto-create default record
		setting = FeishuNotificationSetting{}
		if createErr := DB.Create(&setting).Error; createErr != nil {
			return nil, createErr
		}
	}
	return &setting, nil
}

// SaveFeishuNotificationSetting persists changes.
func SaveFeishuNotificationSetting(s *FeishuNotificationSetting) error {
	return DB.Save(s).Error
}

// ListAccessRequests returns one page of access requests (admin view), newest first.
func ListAccessRequests(page, size int) ([]AccessRequest, int64, error) {
	var reqs []AccessRequest
	var total int64
	query := DB.Model(&AccessRequest{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("created_at desc").Offset((page - 1) * size).Limit(size).Find(&reqs).Error
	return reqs, total, err
}

// ListMyAccessRequests returns requests submitted by a specific user.
func ListMyAccessRequests(userID uint) ([]AccessRequest, error) {
	var reqs []AccessRequest
	err := DB.Where("requester_id = ?", userID).Order("created_at desc").Find(&reqs).Error
	return reqs, err
}

// GetAccessRequest fetches a single request by ID.
func GetAccessRequest(id uint) (*AccessRequest, error) {
	var req AccessRequest
	err := DB.First(&req, id).Error
	return &req, err
}

// CreateAccessRequest inserts a new request record.
func CreateAccessRequest(req *AccessRequest) error {
	return DB.Create(req).Error
}

// SaveAccessRequest persists changes to an existing request.
func SaveAccessRequest(req *AccessRequest) error {
	return DB.Save(req).Error
}

// ListExpiredActiveRequests returns approved requests whose ExpiresAt has passed.
func ListExpiredActiveRequests() ([]AccessRequest, error) {
	var reqs []AccessRequest
	err := DB.Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?",
		AccessRequestApproved, time.Now()).Find(&reqs).Error
	return reqs, err
}

// ExpireAccessRequestAndQueueSummary atomically changes an approved request to
// expired and creates its durable summary job. When onlyIfDue is true, the
// expiry timestamp must have passed. The conditional update makes the
// transition safe when several stateless Kite replicas run the worker.
func ExpireAccessRequestAndQueueSummary(id uint, now time.Time, onlyIfDue bool) (bool, error) {
	query := DB.Model(&AccessRequest{}).
		Where("id = ? AND status = ?", id, AccessRequestApproved)
	if onlyIfDue {
		query = query.Where("expires_at IS NOT NULL AND expires_at <= ?", now)
	}
	result := query.Updates(map[string]interface{}{
		"status":                    AccessRequestExpired,
		"role_id":                   nil,
		"ended_at":                  now,
		"summary_status":            AccessSummaryPending,
		"summary_attempts":          0,
		"summary_ai_attempts":       0,
		"summary_delivery_attempts": 0,
		"summary_last_error":        "",
		"summary_next_retry_at":     nil,
		"summary_claimed_at":        nil,
		"summary_claim_token":       "",
		"summary_completed_at":      nil,
		"summary_message_id":        "",
		"summary_content":           "",
		"summary_stats":             "",
		"expiring_soon_notified":    true,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// SaveAccessSummaryContent persists a successfully generated summary before
// attempting Feishu delivery. Retried or reclaimed jobs can then deliver the
// same content without calling AI again.
func SaveAccessSummaryContent(id uint, claimToken, summary, stats string) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_content":    summary,
			"summary_stats":      stats,
			"summary_last_error": "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ListExpiringSoonRequests returns approved requests that will expire within the
// given threshold and have not yet been notified.
func ListExpiringSoonRequests(threshold time.Duration) ([]AccessRequest, error) {
	now := time.Now()
	deadline := now.Add(threshold)
	var reqs []AccessRequest
	err := DB.Where("status = ? AND expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ? AND expiring_soon_notified = ?",
		AccessRequestApproved, now, deadline, false).Find(&reqs).Error
	return reqs, err
}

// ClaimExpiringSoonNotification atomically marks a single request as notified
// using a conditional UPDATE (WHERE expiring_soon_notified = false). This
// eliminates the read-modify-write race that caused duplicate "即将到期"
// notifications when multiple worker ticks or replicas ran concurrently.
// Returns true if this caller won the claim, false if another caller already
// claimed it.
func ClaimExpiringSoonNotification(id uint) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND expiring_soon_notified = ?", id, false).
		Update("expiring_soon_notified", true)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListAccessSummaryCandidates returns durable summary jobs that are ready to
// run. A processing job becomes eligible again after staleBefore so a process
// crash cannot leave it stuck forever.
func ListAccessSummaryCandidates(now, staleBefore time.Time, limit int) ([]AccessRequest, error) {
	var reqs []AccessRequest
	err := DB.Where("status = ?", AccessRequestExpired).
		Where(`(
			(summary_status IN ? AND (summary_next_retry_at IS NULL OR summary_next_retry_at <= ?))
			OR
			(summary_status = ? AND (summary_claimed_at IS NULL OR summary_claimed_at <= ?))
		)`, []string{AccessSummaryPending, AccessSummaryFailed}, now, AccessSummaryProcessing, staleBefore).
		Order("expires_at ASC").
		Limit(limit).
		Find(&reqs).Error
	return reqs, err
}

// ClaimAccessSummary atomically assigns one summary job to a worker. The claim
// token prevents a stale worker from completing a job after another replica
// has already reclaimed it.
func ClaimAccessSummary(id uint, claimToken string, now, staleBefore time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND status = ?", id, AccessRequestExpired).
		Where(`(
			(summary_status IN ? AND (summary_next_retry_at IS NULL OR summary_next_retry_at <= ?))
			OR
			(summary_status = ? AND (summary_claimed_at IS NULL OR summary_claimed_at <= ?))
		)`, []string{AccessSummaryPending, AccessSummaryFailed}, now, AccessSummaryProcessing, staleBefore).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryProcessing,
			"summary_claimed_at":    now,
			"summary_claim_token":   claimToken,
			"summary_next_retry_at": nil,
			"summary_attempts":      gorm.Expr("summary_attempts + 1"),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummarySent completes a claimed summary job.
func MarkAccessSummarySent(id uint, claimToken, messageID string, completedAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummarySent,
			"summary_completed_at":  completedAt,
			"summary_message_id":    messageID,
			"summary_last_error":    "",
			"summary_next_retry_at": nil,
			"summary_claimed_at":    nil,
			"summary_claim_token":   "",
			"summary_content":       "",
			"summary_stats":         "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummaryDeliveryFailed records one failed delivery independently
// from AI attempts and releases the job for a later retry.
func MarkAccessSummaryDeliveryFailed(id uint, claimToken, lastError string, nextRetryAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":            AccessSummaryFailed,
			"summary_delivery_attempts": gorm.Expr("summary_delivery_attempts + 1"),
			"summary_last_error":        lastError,
			"summary_next_retry_at":     nextRetryAt,
			"summary_claimed_at":        nil,
			"summary_claim_token":       "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummaryFailed releases a claimed job for a later retry.
func MarkAccessSummaryFailed(id uint, claimToken, lastError string, nextRetryAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryFailed,
			"summary_last_error":    lastError,
			"summary_next_retry_at": nextRetryAt,
			"summary_claimed_at":    nil,
			"summary_claim_token":   "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummaryAIFailed persists one failed AI attempt and releases the
// delivery job for retry.
func MarkAccessSummaryAIFailed(id uint, claimToken, lastError string, nextRetryAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryFailed,
			"summary_ai_attempts":   gorm.Expr("summary_ai_attempts + 1"),
			"summary_last_error":    lastError,
			"summary_next_retry_at": nextRetryAt,
			"summary_claimed_at":    nil,
			"summary_claim_token":   "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummaryNotificationFailed persists the terminal failure reason
// while retrying only the Feishu failure notification itself.
func MarkAccessSummaryNotificationFailed(id uint, claimToken, reason string, aiAttempts int, nextRetryAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryFailed,
			"summary_ai_attempts":   aiAttempts,
			"summary_last_error":    reason,
			"summary_next_retry_at": nextRetryAt,
			"summary_claimed_at":    nil,
			"summary_claim_token":   "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkAccessSummaryFailedNotified records that a terminal failure was reported
// to Feishu. It is intentionally not eligible for further summary retries.
func MarkAccessSummaryFailedNotified(id uint, claimToken, messageID, reason string, aiAttempts int, completedAt time.Time) (bool, error) {
	result := DB.Model(&AccessRequest{}).
		Where("id = ? AND summary_status = ? AND summary_claim_token = ?", id, AccessSummaryProcessing, claimToken).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryFailedNotified,
			"summary_ai_attempts":   aiAttempts,
			"summary_last_error":    reason,
			"summary_message_id":    messageID,
			"summary_completed_at":  completedAt,
			"summary_next_retry_at": nil,
			"summary_claimed_at":    nil,
			"summary_claim_token":   "",
			"summary_content":       "",
			"summary_stats":         "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// RetryFailedAccessSummariesNow makes failed jobs immediately eligible after
// an administrator repairs the Feishu configuration.
func RetryFailedAccessSummariesNow() error {
	return DB.Model(&AccessRequest{}).
		Where("status = ? AND summary_status = ?", AccessRequestExpired, AccessSummaryFailed).
		Updates(map[string]interface{}{
			"summary_status":        AccessSummaryPending,
			"summary_next_retry_at": nil,
		}).Error
}
