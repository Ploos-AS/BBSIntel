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

type failingAdapter struct {
	name string
}

func (a failingAdapter) Name() string { return a.name }
func (a failingAdapter) Fetch(context.Context) ([]source.Entry, error) {
	return nil, errors.New("temporary source failure")
}

func TestImportMarksMissingSourceEntriesAndReactivatesThem(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	full := staticAdapter{name: "catalog", entries: []source.Entry{
		{Source: "catalog", SourceKey: "a", Name: "A", Protocol: "telnet", Hostname: "a.example", Port: 23},
		{Source: "catalog", SourceKey: "b", Name: "B", Protocol: "telnet", Hostname: "b.example", Port: 23},
	}}
	if _, err := ingest.Import(context.Background(), s.DB, full); err != nil {
		t.Fatal(err)
	}

	partial := staticAdapter{name: "catalog", entries: []source.Entry{
		{Source: "catalog", SourceKey: "a", Name: "A", Protocol: "telnet", Hostname: "a.example", Port: 23},
	}}
	if _, err := ingest.Import(context.Background(), s.DB, partial); err != nil {
		t.Fatal(err)
	}

	var active int
	var missingSince string
	if err := s.DB.QueryRow(`SELECT active,missing_since FROM source_entry WHERE source='catalog' AND source_key='b'`).Scan(&active, &missingSince); err != nil {
		t.Fatal(err)
	}
	if active != 0 || missingSince == "" {
		t.Fatalf("missing source state active=%d missing_since=%q", active, missingSince)
	}
	var disappeared int
	_ = s.DB.QueryRow(`SELECT count(*) FROM change_event WHERE kind='source_disappeared' AND source='catalog'`).Scan(&disappeared)
	if disappeared != 1 {
		t.Fatalf("source_disappeared events=%d, want 1", disappeared)
	}

	if _, err := ingest.Import(context.Background(), s.DB, full); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT active,missing_since FROM source_entry WHERE source='catalog' AND source_key='b'`).Scan(&active, &missingSince); err != nil {
		t.Fatal(err)
	}
	if active != 1 || missingSince != "" {
		t.Fatalf("returned source state active=%d missing_since=%q", active, missingSince)
	}
	var returned int
	_ = s.DB.QueryRow(`SELECT count(*) FROM change_event WHERE kind='source_returned' AND source='catalog'`).Scan(&returned)
	if returned != 1 {
		t.Fatalf("source_returned events=%d, want 1", returned)
	}
}

func TestImportFailureDoesNotMarkEntriesMissing(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	full := staticAdapter{name: "catalog", entries: []source.Entry{{
		Source: "catalog", SourceKey: "a", Name: "A", Protocol: "telnet", Hostname: "a.example", Port: 23,
	}}}
	if _, err := ingest.Import(context.Background(), s.DB, full); err != nil {
		t.Fatal(err)
	}
	if _, err := ingest.Import(context.Background(), s.DB, failingAdapter{name: "catalog"}); err == nil {
		t.Fatal("expected fetch failure")
	}

	var active int
	if err := s.DB.QueryRow(`SELECT active FROM source_entry WHERE source='catalog' AND source_key='a'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active=%d after fetch failure, want 1", active)
	}
}

func TestImportRejectsEmptySnapshot(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := ingest.Import(context.Background(), s.DB, staticAdapter{name: "catalog"}); err == nil {
		t.Fatal("expected empty snapshot rejection")
	}
}
