package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zxh326/kite/pkg/model"
)

func TestBuildAccessUsageSummaryWithoutChanges(t *testing.T) {
	stats := buildUsageStats(nil)
	summary := buildNoChangeAccessUsageSummary()

	if !strings.Contains(stats, "总操作数：0") || !strings.Contains(stats, "操作类型：无资源变更") {
		t.Fatalf("zero-change stats are incomplete: %q", stats)
	}
	if !strings.Contains(summary, "未检测到资源变更操作") {
		t.Fatalf("zero-change summary is missing: %q", summary)
	}
}

func TestGenerateAndSendAccessUsageSummarySendsWithoutChanges(t *testing.T) {
	originalCollect := collectAccessUsageHistoryForSummary
	originalPersist := persistAccessUsageSummaryForSummary
	originalSend := sendAccessUsageSummaryCard
	t.Cleanup(func() {
		collectAccessUsageHistoryForSummary = originalCollect
		persistAccessUsageSummaryForSummary = originalPersist
		sendAccessUsageSummaryCard = originalSend
	})

	collectAccessUsageHistoryForSummary = func(*model.AccessRequest) ([]model.ResourceHistory, error) {
		return nil, nil
	}
	persistAccessUsageSummaryForSummary = func(id uint, claimToken, summary, stats string) (bool, error) {
		if id != 99 || claimToken != "claim" || summary == "" || stats == "" {
			t.Fatalf("unexpected persisted summary: id=%d token=%q summary=%q stats=%q", id, claimToken, summary, stats)
		}
		return true, nil
	}
	sent := false
	sendAccessUsageSummaryCard = func(_ *model.AccessRequest, summary, stats string) (string, error) {
		sent = true
		if !strings.Contains(summary, "未检测到资源变更操作") || !strings.Contains(stats, "总操作数：0") {
			t.Fatalf("unexpected zero-change card: summary=%q stats=%q", summary, stats)
		}
		return "summary-message", nil
	}

	result, err := generateAndSendAccessUsageSummary(&model.AccessRequest{
		Model: model.Model{ID: 99}, MessageID: "request-message", SummaryClaimToken: "claim",
	})
	if err != nil {
		t.Fatalf("generateAndSendAccessUsageSummary() error = %v", err)
	}
	if !sent || result.MessageID != "summary-message" || result.FailedNotified {
		t.Fatalf("zero-change summary was not sent: sent=%v result=%+v", sent, result)
	}
}

func TestPersistedSummarySkipsHistoryAndAIOnDeliveryRetry(t *testing.T) {
	originalCollect := collectAccessUsageHistoryForSummary
	originalGenerate := generateAccessUsageAnalysisForSummary
	originalPersist := persistAccessUsageSummaryForSummary
	originalSend := sendAccessUsageSummaryCard
	t.Cleanup(func() {
		collectAccessUsageHistoryForSummary = originalCollect
		generateAccessUsageAnalysisForSummary = originalGenerate
		persistAccessUsageSummaryForSummary = originalPersist
		sendAccessUsageSummaryCard = originalSend
	})

	collectAccessUsageHistoryForSummary = func(*model.AccessRequest) ([]model.ResourceHistory, error) {
		t.Fatal("history must not be queried when generated content is persisted")
		return nil, nil
	}
	generateAccessUsageAnalysisForSummary = func(*model.AccessRequest, []model.ResourceHistory, string) (string, error) {
		t.Fatal("AI must not be called again on a delivery retry")
		return "", nil
	}
	persistAccessUsageSummaryForSummary = func(uint, string, string, string) (bool, error) {
		t.Fatal("persisted content must not be written again")
		return false, nil
	}
	sendAccessUsageSummaryCard = func(_ *model.AccessRequest, summary, stats string) (string, error) {
		if summary != "saved summary" || stats != "saved stats" {
			t.Fatalf("unexpected persisted delivery: summary=%q stats=%q", summary, stats)
		}
		return "", fmt.Errorf("temporary feishu failure")
	}

	_, err := generateAndSendAccessUsageSummary(&model.AccessRequest{
		Model: model.Model{ID: 102}, MessageID: "request-message",
		SummaryContent: "saved summary", SummaryStats: "saved stats", SummaryAIAttempts: 2,
	})
	var deliveryErr *accessSummaryDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("delivery retry returned %T, want accessSummaryDeliveryError: %v", err, err)
	}
}

func TestAccessUsageSummaryRetriesAIThenNotifiesFailure(t *testing.T) {
	originalCollect := collectAccessUsageHistoryForSummary
	originalGenerate := generateAccessUsageAnalysisForSummary
	originalNotify := sendAISummaryFailureForSummary
	t.Cleanup(func() {
		collectAccessUsageHistoryForSummary = originalCollect
		generateAccessUsageAnalysisForSummary = originalGenerate
		sendAISummaryFailureForSummary = originalNotify
	})

	collectAccessUsageHistoryForSummary = func(*model.AccessRequest) ([]model.ResourceHistory, error) {
		return []model.ResourceHistory{{OperationType: "update", Success: true}}, nil
	}
	generateAccessUsageAnalysisForSummary = func(*model.AccessRequest, []model.ResourceHistory, string) (string, error) {
		return "", fmt.Errorf("provider timeout\nupstream unavailable")
	}

	firstTry := &model.AccessRequest{Model: model.Model{ID: 100}, MessageID: "request-message"}
	if _, err := generateAndSendAccessUsageSummary(firstTry); err == nil {
		t.Fatal("first AI failure must be retried")
	} else {
		var aiErr *accessSummaryAIError
		if !errors.As(err, &aiErr) || !strings.Contains(aiErr.Error(), "provider timeout") {
			t.Fatalf("unexpected first AI error: %v", err)
		}
	}

	notified := false
	sendAISummaryFailureForSummary = func(_ *model.AccessRequest, reason string, attempts int) (string, error) {
		notified = true
		if attempts != 3 || reason != "provider timeout upstream unavailable" {
			t.Fatalf("unexpected failure notice: attempts=%d reason=%q", attempts, reason)
		}
		return "failure-reply", nil
	}
	thirdTry := &model.AccessRequest{
		Model: model.Model{ID: 100}, MessageID: "request-message", SummaryAIAttempts: 2,
	}
	result, err := generateAndSendAccessUsageSummary(thirdTry)
	if err != nil {
		t.Fatalf("third AI failure notification error = %v", err)
	}
	if !notified || !result.FailedNotified || result.MessageID != "failure-reply" || result.AIAttempts != 3 {
		t.Fatalf("terminal AI failure was not recorded: notified=%v result=%+v", notified, result)
	}
}

func TestMissingMessageIDSendsFailureNoticeNotSummaryCard(t *testing.T) {
	originalMissing := sendMissingMessageIDForSummary
	originalSend := sendAccessUsageSummaryCard
	t.Cleanup(func() {
		sendMissingMessageIDForSummary = originalMissing
		sendAccessUsageSummaryCard = originalSend
	})

	sendAccessUsageSummaryCard = func(*model.AccessRequest, string, string) (string, error) {
		t.Fatal("summary card must not be sent without the original message_id")
		return "", nil
	}
	sendMissingMessageIDForSummary = func(req *model.AccessRequest, reason string) (string, error) {
		if req.ID != 101 || !strings.Contains(reason, "message_id") {
			t.Fatalf("unexpected missing message_id notice: request=%d reason=%q", req.ID, reason)
		}
		return "group-alert", nil
	}

	result, err := generateAndSendAccessUsageSummary(&model.AccessRequest{Model: model.Model{ID: 101}})
	if err != nil {
		t.Fatalf("missing message_id notification error = %v", err)
	}
	if !result.FailedNotified || result.MessageID != "group-alert" {
		t.Fatalf("missing message_id was not reported: %+v", result)
	}
}

func TestAccessRequestGrantedHoursIncludesRenewal(t *testing.T) {
	approvedAt := time.Date(2026, 7, 19, 10, 0, 0, 0, time.Local)
	expiresAt := approvedAt.Add(3*time.Hour + 10*time.Minute)
	req := &model.AccessRequest{
		DurationHours: 1,
		ApprovedAt:    &approvedAt,
		ExpiresAt:     &expiresAt,
	}
	if got := accessRequestGrantedHours(req); got != 4 {
		t.Fatalf("accessRequestGrantedHours() = %d, want 4", got)
	}
}

func TestAccessRequestGrantedHoursUsesActualEndTime(t *testing.T) {
	approvedAt := time.Date(2026, 7, 19, 10, 0, 0, 0, time.Local)
	expiresAt := approvedAt.Add(5 * time.Hour)
	endedAt := approvedAt.Add(70 * time.Minute)
	req := &model.AccessRequest{
		DurationHours: 5,
		ApprovedAt:    &approvedAt,
		ExpiresAt:     &expiresAt,
		EndedAt:       &endedAt,
	}
	if got := accessRequestGrantedHours(req); got != 2 {
		t.Fatalf("accessRequestGrantedHours() = %d, want 2", got)
	}
}

func TestAccessSummaryRetryDelayIsCapped(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Minute},
		{attempt: 2, want: 2 * time.Minute},
		{attempt: 7, want: time.Hour},
		{attempt: 20, want: time.Hour},
	}
	for _, test := range tests {
		if got := accessSummaryRetryDelay(test.attempt); got != test.want {
			t.Fatalf("accessSummaryRetryDelay(%d) = %s, want %s", test.attempt, got, test.want)
		}
	}
}
