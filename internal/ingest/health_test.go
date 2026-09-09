package ingest_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/ingest"
	"github.com/Ploos-AS/BBSIntel/internal/source"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

type healthFailingAdapter struct{ err error }

func (a healthFailingAdapter) Name() string { return "health-source" }
func (a healthFailingAdapter) Fetch(context.Context) ([]source.Entry, error) {
	return nil, a.err
}

func TestImportRecordsSourceHealthAndRecovery(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	boom := errors.New("directory unavailable")
	if _, err := ingest.Import(context.Background(), s.DB, healthFailingAdapter{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("failure err=%v, want %v", err, boom)
	}

	var failures int
	var lastError string
	if err := s.DB.QueryRow(`SELECT consecutive_failures,last_error FROM source_health WHERE source='health-source'`).Scan(&failures, &lastError); err != nil {
		t.Fatal(err)
	}
	if failures != 1 || lastError != boom.Error() {
		t.Fatalf("failure telemetry failures=%d error=%q", failures, lastError)
	}

	good := staticAdapter{name: "health-source", entries: []source.Entry{{
		Source: "health-source", SourceKey: "one", Name: "Health BBS",
		Protocol: "telnet", Hostname: "health.example", Port: 23,
	}}}
	if n, err := ingest.Import(context.Background(), s.DB, good); err != nil || n != 1 {
		t.Fatalf("recovery import n=%d err=%v", n, err)
	}

	var lastAttempt, lastSuccess string
	var entryCount int
	if err := s.DB.QueryRow(`SELECT last_attempt_at,last_success_at,last_entry_count,consecutive_failures,last_error FROM source_health WHERE source='health-source'`).Scan(
		&lastAttempt, &lastSuccess, &entryCount, &failures, &lastError,
	); err != nil {
		t.Fatal(err)
	}
	if lastAttempt == "" || lastSuccess == "" || entryCount != 1 || failures != 0 || lastError != "" {
		t.Fatalf("recovery telemetry attempt=%q success=%q entries=%d failures=%d error=%q", lastAttempt, lastSuccess, entryCount, failures, lastError)
	}
}
