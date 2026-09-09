package sourcehealth

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		success  time.Time
		failures int
		want     string
	}{
		{"unknown", time.Time{}, 0, "unknown"},
		{"never-success-failed", time.Time{}, 1, "failed"},
		{"fresh", now.Add(-12 * time.Hour), 0, "fresh"},
		{"degraded-age", now.Add(-48 * time.Hour), 0, "degraded"},
		{"degraded-failure", now.Add(-12 * time.Hour), 1, "degraded"},
		{"stale", now.Add(-96 * time.Hour), 0, "stale"},
		{"failed", now.Add(-12 * time.Hour), 3, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(now, tt.success, tt.failures)
			if got.State != tt.want {
				t.Fatalf("state=%q, want %q", got.State, tt.want)
			}
		})
	}
}
