package ai

import (
	"testing"
	"time"
)

func TestConversationSessionStoreIsClusterScoped(t *testing.T) {
	store := &conversationSessionStore{entries: make(map[string]*conversationSession)}
	store.save("session-a", conversationSession{ClusterName: "cluster-a"})

	if _, ok := store.load("session-a", "cluster-a"); !ok {
		t.Fatal("same-cluster session should be available")
	}
	if _, ok := store.load("session-a", "cluster-b"); ok {
		t.Fatal("cross-cluster session must not be returned")
	}

	store.deleteForCluster("session-a", "cluster-b")
	if _, ok := store.load("session-a", "cluster-a"); !ok {
		t.Fatal("cross-cluster delete must not remove the session")
	}
	store.deleteForCluster("session-a", "cluster-a")
	if _, ok := store.load("session-a", "cluster-a"); ok {
		t.Fatal("same-cluster delete should remove the session")
	}
}

func TestConversationSessionStoreRejectsLegacyUnscopedSession(t *testing.T) {
	store := &conversationSessionStore{
		entries: map[string]*conversationSession{
			"legacy": {ExpiresAt: time.Now().Add(time.Minute)},
		},
	}
	if _, ok := store.load("legacy", "cluster-a"); ok {
		t.Fatal("session without a cluster must fail closed")
	}
}
