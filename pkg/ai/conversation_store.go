package ai

import (
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
)

const conversationSessionTTL = 24 * time.Hour

// conversationSession holds the full LLM-format message history for one chat session.
// Stored server-side so the AI always has complete untruncated context.
type conversationSession struct {
	Provider          string
	SystemPrompt      string
	OpenAIMessages    []openai.ChatCompletionMessageParamUnion
	AnthropicMessages []anthropic.MessageParam
	ExpiresAt         time.Time
}

type conversationSessionStore struct {
	mu      sync.RWMutex
	entries map[string]*conversationSession
}

var agentConversationStore = newConversationSessionStore()

func newConversationSessionStore() *conversationSessionStore {
	s := &conversationSessionStore{
		entries: make(map[string]*conversationSession),
	}
	go s.runCleanup()
	return s
}

func (s *conversationSessionStore) load(sessionID string) (*conversationSession, bool) {
	if sessionID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[sessionID]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry, true
}

func (s *conversationSessionStore) save(sessionID string, session conversationSession) {
	if sessionID == "" {
		return
	}
	session.ExpiresAt = time.Now().Add(conversationSessionTTL)
	s.mu.Lock()
	s.entries[sessionID] = &session
	s.mu.Unlock()
}

func (s *conversationSessionStore) delete(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, sessionID)
	s.mu.Unlock()
}

func (s *conversationSessionStore) runCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, entry := range s.entries {
			if now.After(entry.ExpiresAt) {
				delete(s.entries, id)
			}
		}
		s.mu.Unlock()
	}
}
