package ingest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/ingest"
	"github.com/Ploos-AS/BBSIntel/internal/source"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestImportRecordsSourceChanges(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a := staticAdapter{name: "test", entries: []source.Entry{{
		Source: "test", SourceKey: "bbs", Name: "Event BBS", Software: "Mystic",
		Protocol: "telnet", Hostname: "bbs.example", Port: 23,
	}}}
	if _, err := ingest.Import(context.Background(), s.DB, a); err != nil {
		t.Fatal(err)
	}
	a.entries[0].Software = "Synchronet"
	if _, err := ingest.Import(context.Background(), s.DB, a); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := s.DB.QueryRow(`SELECT count(*) FROM change_event WHERE kind='source_changed' AND field='software' AND old_value='Mystic' AND new_value='Synchronet'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("software change events=%d, want 1", count)
	}
}
