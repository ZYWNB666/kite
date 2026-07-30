package model

import (
	"time"
)

type PendingSession struct {
	Model
	ClusterName           string    `json:"cluster_name" gorm:"type:varchar(255)"`
	SessionID             string    `json:"session_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	Provider              string    `json:"provider" gorm:"type:varchar(32);not null"`
	ConversationSessionID string    `json:"conversation_session_id" gorm:"type:varchar(64)"`
	SystemPrompt          string    `json:"system_prompt" gorm:"size:16777215"`
	OpenAIMessages        JSONField `json:"openai_messages" gorm:"size:16777215"`
	AnthropicMessages     JSONField `json:"anthropic_messages" gorm:"size:16777215"`
	ToolCallID            string    `json:"tool_call_id" gorm:"type:varchar(255)"`
	ToolCallName          string    `json:"tool_call_name" gorm:"type:varchar(255)"`
	ToolCallArgs          JSONField `json:"tool_call_args" gorm:"size:16777215"`
	ExpiresAt             time.Time `json:"expires_at" gorm:"index;not null"`
}

func SavePendingSession(session *PendingSession) error {
	return DB.Create(session).Error
}

func GetPendingSession(sessionID string) (*PendingSession, error) {
	var session PendingSession
	if err := DB.Where("session_id = ? AND expires_at > ?", sessionID, time.Now()).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func DeletePendingSession(sessionID string) error {
	return DB.Where("session_id = ?", sessionID).Delete(&PendingSession{}).Error
}

func CleanupExpiredPendingSessions() error {
	return DB.Where("expires_at <= ?", time.Now()).Delete(&PendingSession{}).Error
}
