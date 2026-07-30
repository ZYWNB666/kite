package ai

import "testing"

func TestValidatePendingSessionCluster(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		requestName string
		wantError   bool
	}{
		{name: "same cluster", sessionName: "cluster-a", requestName: "cluster-a"},
		{name: "different cluster", sessionName: "cluster-a", requestName: "cluster-b", wantError: true},
		{name: "legacy session without cluster", requestName: "cluster-a", wantError: true},
		{name: "request without cluster", sessionName: "cluster-a", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePendingSessionCluster(
				pendingSession{ClusterName: tt.sessionName},
				tt.requestName,
			)
			if (err != nil) != tt.wantError {
				t.Fatalf("validatePendingSessionCluster() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
