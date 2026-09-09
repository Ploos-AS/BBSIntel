package sourcehealth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/sourcehealth"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestEvaluateAllRecordsOnlyTransitions(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Date(2026, 9, 9, 2, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.DB.Exec(`INSERT INTO source_health(source,last_success_at,current_state) VALUES(?,?,?)`, "test", lastSuccess, "unknown"); err != nil {
		t.Fatal(err)
	}

	if n, err := sourcehealth.EvaluateAll(context.Background(), s.DB, now); err != nil || n != 1 {
		t.Fatalf("first evaluate n=%d err=%v", n, err)
	}
	assertStateAndEvents(t, s, "fresh", 1)

	if n, err := sourcehealth.EvaluateAll(context.Background(), s.DB, now); err != nil || n != 0 {
		t.Fatalf("duplicate evaluate n=%d err=%v", n, err)
	}
	assertStateAndEvents(t, s, "fresh", 1)

	if n, err := sourcehealth.EvaluateAll(context.Background(), s.DB, now.Add(48*time.Hour)); err != nil || n != 1 {
		t.Fatalf("degraded evaluate n=%d err=%v", n, err)
	}
	assertStateAndEvents(t, s, "degraded", 2)

	if n, err := sourcehealth.EvaluateAll(context.Background(), s.DB, now.Add(80*time.Hour)); err != nil || n != 1 {
		t.Fatalf("stale evaluate n=%d err=%v", n, err)
	}
	assertStateAndEvents(t, s, "stale", 3)
}

func assertStateAndEvents(t *testing.T, s *store.Store, wantState string, wantEvents int) {
	t.Helper()
	var state string
	if err := s.DB.QueryRow(`SELECT current_state FROM source_health WHERE source='test'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != wantState {
		t.Fatalf("state=%q, want %q", state, wantState)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT count(*) FROM change_event WHERE kind='source_health_changed' AND source='test'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != wantEvents {
		t.Fatalf("events=%d, want %d", count, wantEvents)
	}
}
