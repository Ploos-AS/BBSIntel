package ingest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/ingest"
	"github.com/Ploos-AS/BBSIntel/internal/source"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

type staticAdapter struct {
	name    string
	entries []source.Entry
}

func (a staticAdapter) Name() string { return a.name }
func (a staticAdapter) Fetch(context.Context) ([]source.Entry, error) { return a.entries, nil }

func TestImportReusesBBSAcrossSourcesForSameEndpoint(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	first := staticAdapter{name: "one", entries: []source.Entry{{
		Source: "one", SourceKey: "one-key", Name: "Canonical BBS", Software: "Mystic",
		Protocol: "telnet", Hostname: "bbs.example", Port: 23,
	}}}
	second := staticAdapter{name: "two", entries: []source.Entry{{
		Source: "two", SourceKey: "two-key", Name: "Different Directory Name", Software: "Synchronet",
		Protocol: "telnet", Hostname: "BBS.EXAMPLE", Port: 23,
	}}}

	if _, err := ingest.Import(context.Background(), s.DB, first); err != nil {
		t.Fatal(err)
	}
	if _, err := ingest.Import(context.Background(), s.DB, second); err != nil {
		t.Fatal(err)
	}

	var bbsCount, endpointCount, sourceCount int
	_ = s.DB.QueryRow(`SELECT count(*) FROM bbs`).Scan(&bbsCount)
	_ = s.DB.QueryRow(`SELECT count(*) FROM endpoint`).Scan(&endpointCount)
	_ = s.DB.QueryRow(`SELECT count(*) FROM source_entry`).Scan(&sourceCount)
	if bbsCount != 1 || endpointCount != 1 || sourceCount != 2 {
		t.Fatalf("counts bbs=%d endpoint=%d source=%d, want 1/1/2", bbsCount, endpointCount, sourceCount)
	}

	var name, software string
	if err := s.DB.QueryRow(`SELECT name,software FROM bbs`).Scan(&name, &software); err != nil {
		t.Fatal(err)
	}
	if name != "Canonical BBS" || software != "Mystic" {
		t.Fatalf("canonical metadata overwritten: name=%q software=%q", name, software)
	}
}

func TestImportGroupsMultipleEndpointsBySourceKey(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a := staticAdapter{name: "multi", entries: []source.Entry{
		{Source: "multi", SourceKey: "same-bbs", Name: "Multi BBS", Protocol: "telnet", Hostname: "bbs.example", Port: 23},
		{Source: "multi", SourceKey: "same-bbs", Name: "Multi BBS", Protocol: "ssh", Hostname: "bbs.example", Port: 22},
	}}
	if _, err := ingest.Import(context.Background(), s.DB, a); err != nil {
		t.Fatal(err)
	}

	var bbsCount, endpointCount, sourceCount int
	_ = s.DB.QueryRow(`SELECT count(*) FROM bbs`).Scan(&bbsCount)
	_ = s.DB.QueryRow(`SELECT count(*) FROM endpoint`).Scan(&endpointCount)
	_ = s.DB.QueryRow(`SELECT count(*) FROM source_entry`).Scan(&sourceCount)
	if bbsCount != 1 || endpointCount != 2 || sourceCount != 1 {
		t.Fatalf("counts bbs=%d endpoint=%d source=%d, want 1/2/1", bbsCount, endpointCount, sourceCount)
	}
}
