package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAccessRequestSummaryTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:access-summary-%d?mode=memory&cache=shared", time.Now().UnixNano())))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := testDB.AutoMigrate(&AccessRequest{}); err != nil {
		t.Fatalf("migrate access requests: %v", err)
	}
	DB = testDB
	t.Cleanup(func() {
		DB = originalDB
		sqlDB, dbErr := testDB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestAccessSummaryJobSurvivesAndCanBeReclaimed(t *testing.T) {
	setupAccessRequestSummaryTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(-time.Minute)
	roleID := uint(42)
	req := AccessRequest{
		RequesterID:   7,
		Cluster:       "cluster-a",
		Namespace:     "default",
		DurationHours: 1,
		RiskLevel:     "low",
		Status:        AccessRequestApproved,
		ExpiresAt:     &expiresAt,
		RoleID:        &roleID,
	}
	if err := DB.Create(&req).Error; err != nil {
		t.Fatalf("create request: %v", err)
	}

	expired, err := ExpireAccessRequestAndQueueSummary(req.ID, now, true)
	if err != nil || !expired {
		t.Fatalf("expire request: expired=%v err=%v", expired, err)
	}
	if expiredAgain, err := ExpireAccessRequestAndQueueSummary(req.ID, now, true); err != nil || expiredAgain {
		t.Fatalf("second expiry must not reset the durable job: expired=%v err=%v", expiredAgain, err)
	}

	candidates, err := ListAccessSummaryCandidates(now, now.Add(-5*time.Minute), 20)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("list pending summary: len=%d err=%v", len(candidates), err)
	}
	claimed, err := ClaimAccessSummary(req.ID, "worker-a", now, now.Add(-5*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim summary: claimed=%v err=%v", claimed, err)
	}
	if claimedAgain, err := ClaimAccessSummary(req.ID, "worker-b", now, now.Add(-5*time.Minute)); err != nil || claimedAgain {
		t.Fatalf("active claim must be exclusive: claimed=%v err=%v", claimedAgain, err)
	}

	// Simulate a process restart: once the claim is stale, another worker can
	// atomically reclaim it and the old token can no longer complete the job.
	restartTime := now.Add(10 * time.Minute)
	reclaimed, err := ClaimAccessSummary(req.ID, "worker-b", restartTime, restartTime.Add(-5*time.Minute))
	if err != nil || !reclaimed {
		t.Fatalf("reclaim stale summary: claimed=%v err=%v", reclaimed, err)
	}
	if updated, err := MarkAccessSummarySent(req.ID, "worker-a", "stale-message", restartTime); err != nil || updated {
		t.Fatalf("stale worker must not complete reclaimed job: updated=%v err=%v", updated, err)
	}
	if updated, err := MarkAccessSummarySent(req.ID, "worker-b", "summary-message", restartTime); err != nil || !updated {
		t.Fatalf("complete reclaimed job: updated=%v err=%v", updated, err)
	}

	var saved AccessRequest
	if err := DB.First(&saved, req.ID).Error; err != nil {
		t.Fatalf("reload request: %v", err)
	}
	if saved.EndedAt == nil || !saved.EndedAt.Equal(now) {
		t.Fatalf("actual access end was not persisted: endedAt=%v want=%v", saved.EndedAt, now)
	}
	if saved.SummaryStatus != AccessSummarySent || saved.SummaryAttempts != 2 || saved.SummaryMessageID != "summary-message" {
		t.Fatalf("unexpected durable summary state: status=%s attempts=%d message=%s",
			saved.SummaryStatus, saved.SummaryAttempts, saved.SummaryMessageID)
	}
}

func TestAccessSummaryPersistsContentAcrossDeliveryFailure(t *testing.T) {
	setupAccessRequestSummaryTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	req := AccessRequest{
		RequesterID:   9,
		Cluster:       "cluster-a",
		Namespace:     "default",
		DurationHours: 1,
		RiskLevel:     "low",
		Status:        AccessRequestExpired,
		SummaryStatus: AccessSummaryPending,
	}
	if err := DB.Create(&req).Error; err != nil {
		t.Fatalf("create request: %v", err)
	}
	claimed, err := ClaimAccessSummary(req.ID, "delivery-worker", now, now.Add(-5*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim summary: claimed=%v err=%v", claimed, err)
	}
	if updated, err := SaveAccessSummaryContent(req.ID, "delivery-worker", "saved summary", "saved stats"); err != nil || !updated {
		t.Fatalf("save generated summary: updated=%v err=%v", updated, err)
	}
	retryAt := now.Add(time.Minute)
	if updated, err := MarkAccessSummaryDeliveryFailed(req.ID, "delivery-worker", "temporary Feishu failure", retryAt); err != nil || !updated {
		t.Fatalf("mark delivery failed: updated=%v err=%v", updated, err)
	}

	var saved AccessRequest
	if err := DB.First(&saved, req.ID).Error; err != nil {
		t.Fatalf("reload request: %v", err)
	}
	if saved.SummaryContent != "saved summary" || saved.SummaryStats != "saved stats" {
		t.Fatalf("generated content was lost: summary=%q stats=%q", saved.SummaryContent, saved.SummaryStats)
	}
	if saved.SummaryDeliveryAttempts != 1 || saved.SummaryAIAttempts != 0 {
		t.Fatalf("retry counters are coupled: delivery=%d ai=%d", saved.SummaryDeliveryAttempts, saved.SummaryAIAttempts)
	}
}

func TestFailedAccessSummaryWaitsUntilRetryTime(t *testing.T) {
	setupAccessRequestSummaryTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	req := AccessRequest{
		RequesterID:   8,
		Cluster:       "cluster-a",
		Namespace:     "default",
		DurationHours: 1,
		RiskLevel:     "low",
		Status:        AccessRequestExpired,
		SummaryStatus: AccessSummaryPending,
	}
	if err := DB.Create(&req).Error; err != nil {
		t.Fatalf("create request: %v", err)
	}
	claimed, err := ClaimAccessSummary(req.ID, "worker", now, now.Add(-5*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim summary: claimed=%v err=%v", claimed, err)
	}
	retryAt := now.Add(5 * time.Minute)
	if updated, err := MarkAccessSummaryAIFailed(req.ID, "worker", "temporary AI failure", retryAt); err != nil || !updated {
		t.Fatalf("mark AI failed: updated=%v err=%v", updated, err)
	}

	beforeRetry, err := ListAccessSummaryCandidates(now, now.Add(-5*time.Minute), 20)
	if err != nil || len(beforeRetry) != 0 {
		t.Fatalf("job retried too early: len=%d err=%v", len(beforeRetry), err)
	}
	if err := RetryFailedAccessSummariesNow(); err != nil {
		t.Fatalf("wake failed summaries: %v", err)
	}
	afterConfigRepair, err := ListAccessSummaryCandidates(now, now.Add(-5*time.Minute), 20)
	if err != nil || len(afterConfigRepair) != 1 {
		t.Fatalf("job not available after configuration repair: len=%d err=%v", len(afterConfigRepair), err)
	}

	claimed, err = ClaimAccessSummary(req.ID, "worker-2", now, now.Add(-5*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("reclaim summary: claimed=%v err=%v", claimed, err)
	}
	if updated, err := MarkAccessSummaryFailedNotified(req.ID, "worker-2", "failure-notice", "AI failed three times", 3, now); err != nil || !updated {
		t.Fatalf("mark failure notified: updated=%v err=%v", updated, err)
	}
	var saved AccessRequest
	if err := DB.First(&saved, req.ID).Error; err != nil {
		t.Fatalf("reload failed-notified request: %v", err)
	}
	if saved.SummaryStatus != AccessSummaryFailedNotified || saved.SummaryAIAttempts != 3 || saved.SummaryMessageID != "failure-notice" {
		t.Fatalf("unexpected failure notification state: status=%s aiAttempts=%d message=%s",
			saved.SummaryStatus, saved.SummaryAIAttempts, saved.SummaryMessageID)
	}
}
