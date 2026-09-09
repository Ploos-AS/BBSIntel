package publicstats

import (
	"context"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestRefreshRuntimeSnapshot(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/runtime.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.DB.ExecContext(ctx, `INSERT INTO bbs(id,name,software,country) VALUES
(1,'One','Synchronet','US'),(2,'Two','Mystic','NO')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO endpoint(id,bbs_id,protocol,hostname,port) VALUES
(1,1,'telnet','one.test',23),(2,2,'ssh','two.test',22)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,checked_at,status,detected_software,software_confidence) VALUES
(1,'2026-09-09 10:00:00','offline','',0),
(1,'2026-09-09 11:00:00','online','Synchronet',1),
(2,'2026-09-09 11:00:00','tcp_only','Synchronet',0.8)`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if err := RefreshRuntime(ctx, s.DB, now); err != nil {
		t.Fatal(err)
	}
	got, err := Runtime(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if got.BBS != 2 || got.Endpoints != 2 || got.Probes != 3 {
		t.Fatalf("unexpected counts: %#v", got)
	}
	if got.Status["online"] != 1 || got.Status["tcp_only"] != 1 {
		t.Fatalf("unexpected status snapshot: %#v", got.Status)
	}
	if got.SoftwareMismatches != 1 {
		t.Fatalf("expected one software mismatch, got %d", got.SoftwareMismatches)
	}
	if got.RefreshedAt != "2026-09-09 12:00:00" {
		t.Fatalf("unexpected refreshed_at %q", got.RefreshedAt)
	}

	// Public reads stay stable until the worker explicitly refreshes again.
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,checked_at,status) VALUES(1,'2026-09-09 12:30:00','offline')`); err != nil {
		t.Fatal(err)
	}
	stable, err := Runtime(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if stable.Probes != 3 || stable.Status["online"] != 1 {
		t.Fatalf("snapshot unexpectedly changed without refresh: %#v", stable)
	}
}
