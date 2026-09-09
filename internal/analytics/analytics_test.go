package analytics_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/analytics"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestForBBSAndHistory(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res, err := s.DB.Exec(`INSERT INTO bbs(name,created_at) VALUES('Test BBS','2026-09-01 00:00:00')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, _ := res.LastInsertId()
	if _, err := s.DB.Exec(`INSERT INTO source_entry(bbs_id,source,source_key,last_seen) VALUES(?,?,?,?)`, bbsID, "test", "test", "2026-09-09 10:00:00"); err != nil {
		t.Fatal(err)
	}
	res, err = s.DB.Exec(`INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, "telnet", "bbs.example", 23)
	if err != nil {
		t.Fatal(err)
	}
	endpointID, _ := res.LastInsertId()

	probes := []struct {
		at     string
		status string
	}{
		{"2026-09-08 13:00:00", "online"},
		{"2026-09-08 14:00:00", "offline"},
		{"2026-09-08 15:00:00", "online"},
		{"2026-09-09 11:00:00", "online"},
	}
	for _, p := range probes {
		if _, err := s.DB.Exec(`INSERT INTO probe_result(endpoint_id,checked_at,status) VALUES(?,?,?)`, endpointID, p.at, p.status); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	summary, err := analytics.ForBBS(context.Background(), s.DB, "1", now)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FirstSeen != "2026-09-01 00:00:00" || summary.LastSeen != "2026-09-09 10:00:00" {
		t.Fatalf("unexpected seen bounds: %#v", summary)
	}
	if summary.StatusChanges != 2 {
		t.Fatalf("status changes=%d, want 2", summary.StatusChanges)
	}
	if summary.Uptime24h.Checks != 4 || summary.Uptime24h.OnlineChecks != 3 || summary.Uptime24h.UptimePct != 75 {
		t.Fatalf("24h uptime=%#v, want 3/4 = 75%%", summary.Uptime24h)
	}
	if summary.LastOnline != "2026-09-09 11:00:00" {
		t.Fatalf("last online=%q", summary.LastOnline)
	}

	history, err := analytics.History(context.Background(), s.DB, "1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].CheckedAt != "2026-09-09 11:00:00" || history[1].Status != "online" {
		t.Fatalf("unexpected history: %#v", history)
	}
}
