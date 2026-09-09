package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestRefreshAndPrune(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO bbs(id,name) VALUES(1,'Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO endpoint(id,bbs_id,protocol,hostname,port) VALUES(1,1,'telnet','example.test',23)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ at, status string; ms any }{
		{"2026-09-08 10:05:00", "online", 10},
		{"2026-09-08 10:35:00", "telnet_only", 20},
		{"2026-09-08 11:05:00", "offline", nil},
		{"2026-09-09 10:05:00", "tcp_only", 30},
	} {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,checked_at,status,connect_ms) VALUES(?,?,?,?)`, 1, row.at, row.status, row.ms); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if err := Refresh(ctx, s.DB, now); err != nil {
		t.Fatal(err)
	}

	var checks, online, telnetOnly, tcpOnly, offline int
	if err := s.DB.QueryRowContext(ctx, `SELECT checks,online_checks,telnet_only_checks,tcp_only_checks,offline_checks FROM endpoint_daily WHERE endpoint_id=1 AND bucket_start='2026-09-08 00:00:00'`).Scan(&checks, &online, &telnetOnly, &tcpOnly, &offline); err != nil {
		t.Fatal(err)
	}
	if checks != 3 || online != 1 || telnetOnly != 1 || tcpOnly != 0 || offline != 1 {
		t.Fatalf("unexpected daily rollup: checks=%d online=%d telnet=%d tcp=%d offline=%d", checks, online, telnetOnly, tcpOnly, offline)
	}

	deleted, err := PruneRaw(ctx, s.DB, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 pruned rows, got %d", deleted)
	}
	var daily int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM endpoint_daily`).Scan(&daily); err != nil {
		t.Fatal(err)
	}
	if daily != 2 {
		t.Fatalf("expected daily history to survive pruning, got %d rows", daily)
	}
}
