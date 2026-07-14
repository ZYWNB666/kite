package model

import (
	"encoding/json"
	"time"
)

const (
	AccessRequestPending   = "pending"
	AccessRequestApproved  = "approved"
	AccessRequestRejected  = "rejected"
	AccessRequestWithdrawn = "withdrawn"
	AccessRequestExpired   = "expired"
)

// AccessRequest records a namespace permission request submitted by a user.
type AccessRequest struct {
	Model
	RequesterID          uint       `json:"requesterId" gorm:"index;not null"`
	RequesterName        string     `json:"requesterName" gorm:"type:varchar(100)"`
	Cluster              string     `json:"cluster" gorm:"type:varchar(255)"`
	Namespace            string     `json:"namespace" gorm:"type:varchar(255);not null"`
	DurationHours        int        `json:"durationHours" gorm:"not null"`
	RiskLevel            string     `json:"riskLevel" gorm:"type:varchar(50);not null;default:'low'"`
	Reason               string     `json:"reason" gorm:"type:text"`
	ApproverUID          string     `json:"approverUid" gorm:"type:varchar(255)"` // Feishu open_id of approver
	ApproverName         string     `json:"approverName" gorm:"type:varchar(100)"`
	Status               string     `json:"status" gorm:"type:varchar(20);index;not null;default:'pending'"`
	ExpiresAt            *time.Time `json:"expiresAt"`
	ApprovedAt           *time.Time `json:"approvedAt"`                                                      // when the request was approved (for usage query)
	ExpiringSoonNotified bool       `json:"expiringSoonNotified" gorm:"type:boolean;not null;default:false"` // whether the expiring-soon notification has been sent
	MessageID            string     `json:"messageId" gorm:"type:varchar(255)"`                              // Feishu message_id for card update
	RoleID               *uint      `json:"roleId"`                                                          // temp role ID, deleted on revoke/expire
	ReviewNote           string     `json:"reviewNote" gorm:"type:text"`
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

// ListAccessRequests returns all access requests (admin view), newest first.
func ListAccessRequests() ([]AccessRequest, error) {
	var reqs []AccessRequest
	err := DB.Order("created_at desc").Find(&reqs).Error
	return reqs, err
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
