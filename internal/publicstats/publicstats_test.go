package publicstats

import (
	"context"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestRefreshDimensionsAndWholeBBSPresence(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.DB.ExecContext(ctx, `INSERT INTO bbs(id,name,software,country,created_at) VALUES
(1,'One','Synchronet','US','2026-09-07 10:00:00'),
(2,'Two','Mystic','NO','2026-09-08 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO endpoint(id,bbs_id,protocol,hostname,port) VALUES
(1,1,'telnet','one.test',23),(2,1,'ssh','one.test',22),(3,2,'telnet','two.test',23)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO source_entry(bbs_id,source,source_key,active) VALUES
(1,'telnetbbsguide','one-tbg',1),(1,'synchronet','one-sync',1),(2,'telnetbbsguide','two-tbg',1)`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if err := Refresh(ctx, s.DB, now); err != nil {
		t.Fatal(err)
	}

	protocols, err := Dimensions(ctx, s.DB, "protocol")
	if err != nil {
		t.Fatal(err)
	}
	if len(protocols) != 2 || protocols[0].Value != "telnet" || protocols[0].Count != 2 {
		t.Fatalf("unexpected protocol dimensions: %#v", protocols)
	}
	sources, err := Dimensions(ctx, s.DB, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Value != "telnetbbsguide" || sources[0].Count != 2 {
		t.Fatalf("unexpected source dimensions: %#v", sources)
	}

	// Losing one of two sources must not mark the BBS disappeared.
	if _, err := s.DB.ExecContext(ctx, `UPDATE source_entry SET active=0 WHERE source='synchronet' AND bbs_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(ctx, s.DB, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var disappeared int
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT disappeared_bbs FROM public_lifecycle_daily WHERE day='2026-09-09'),0)`).Scan(&disappeared); err != nil {
		t.Fatal(err)
	}
	if disappeared != 0 {
		t.Fatalf("expected no disappearance while one source remains, got %d", disappeared)
	}

	if _, err := s.DB.ExecContext(ctx, `UPDATE source_entry SET active=0 WHERE bbs_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(ctx, s.DB, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT disappeared_bbs FROM public_lifecycle_daily WHERE day='2026-09-09'`).Scan(&disappeared); err != nil {
		t.Fatal(err)
	}
	if disappeared != 1 {
		t.Fatalf("expected one whole-BBS disappearance, got %d", disappeared)
	}

	if _, err := s.DB.ExecContext(ctx, `UPDATE source_entry SET active=1 WHERE bbs_id=1 AND source='synchronet'`); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(ctx, s.DB, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var returned int
	if err := s.DB.QueryRowContext(ctx, `SELECT returned_bbs FROM public_lifecycle_daily WHERE day='2026-09-09'`).Scan(&returned); err != nil {
		t.Fatal(err)
	}
	if returned != 1 {
		t.Fatalf("expected one return, got %d", returned)
	}

	lifecycle, err := Lifecycle(ctx, s.DB, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle) != 3 {
		t.Fatalf("expected three lifecycle days including backfilled creation dates, got %#v", lifecycle)
	}
}
