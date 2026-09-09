package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{DB: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA foreign_keys=ON;
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS bbs (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL,
 software TEXT NOT NULL DEFAULT '',
 country TEXT NOT NULL DEFAULT '',
 description TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS endpoint (
 id INTEGER PRIMARY KEY,
 bbs_id INTEGER NOT NULL REFERENCES bbs(id) ON DELETE CASCADE,
 protocol TEXT NOT NULL,
 hostname TEXT NOT NULL,
 port INTEGER NOT NULL,
 UNIQUE(protocol, hostname, port)
);
CREATE TABLE IF NOT EXISTS source_entry (
 id INTEGER PRIMARY KEY,
 bbs_id INTEGER NOT NULL REFERENCES bbs(id) ON DELETE CASCADE,
 source TEXT NOT NULL,
 source_key TEXT NOT NULL,
 source_url TEXT NOT NULL DEFAULT '',
 reported_name TEXT NOT NULL DEFAULT '',
 reported_software TEXT NOT NULL DEFAULT '',
 reported_country TEXT NOT NULL DEFAULT '',
 reported_description TEXT NOT NULL DEFAULT '',
 last_seen TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 active INTEGER NOT NULL DEFAULT 1,
 missing_since TEXT NOT NULL DEFAULT '',
 UNIQUE(source, source_key)
);
CREATE TABLE IF NOT EXISTS source_health (
 source TEXT PRIMARY KEY,
 last_attempt_at TEXT NOT NULL DEFAULT '',
 last_success_at TEXT NOT NULL DEFAULT '',
 last_failure_at TEXT NOT NULL DEFAULT '',
 last_duration_ms INTEGER NOT NULL DEFAULT 0,
 last_entry_count INTEGER NOT NULL DEFAULT 0,
 consecutive_failures INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 current_state TEXT NOT NULL DEFAULT 'unknown',
 state_changed_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS probe_result (
 id INTEGER PRIMARY KEY,
 endpoint_id INTEGER NOT NULL REFERENCES endpoint(id) ON DELETE CASCADE,
 checked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 status TEXT NOT NULL,
 connect_ms INTEGER,
 banner_bytes INTEGER NOT NULL DEFAULT 0,
 banner_sha256 TEXT NOT NULL DEFAULT '',
 banner_preview TEXT NOT NULL DEFAULT '',
 detected_software TEXT NOT NULL DEFAULT '',
 software_confidence REAL NOT NULL DEFAULT 0,
 software_evidence TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS change_event (
 id INTEGER PRIMARY KEY,
 bbs_id INTEGER REFERENCES bbs(id) ON DELETE CASCADE,
 endpoint_id INTEGER REFERENCES endpoint(id) ON DELETE CASCADE,
 occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 kind TEXT NOT NULL,
 source TEXT NOT NULL DEFAULT '',
 field TEXT NOT NULL DEFAULT '',
 old_value TEXT NOT NULL DEFAULT '',
 new_value TEXT NOT NULL DEFAULT '',
 detail TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS endpoint_hourly (
 endpoint_id INTEGER NOT NULL REFERENCES endpoint(id) ON DELETE CASCADE,
 bucket_start TEXT NOT NULL,
 checks INTEGER NOT NULL,
 online_checks INTEGER NOT NULL,
 telnet_only_checks INTEGER NOT NULL,
 tcp_only_checks INTEGER NOT NULL,
 offline_checks INTEGER NOT NULL,
 dns_fail_checks INTEGER NOT NULL,
 connect_ms_sum INTEGER NOT NULL,
 connect_ms_count INTEGER NOT NULL,
 PRIMARY KEY(endpoint_id,bucket_start)
);
CREATE TABLE IF NOT EXISTS endpoint_daily (
 endpoint_id INTEGER NOT NULL REFERENCES endpoint(id) ON DELETE CASCADE,
 bucket_start TEXT NOT NULL,
 checks INTEGER NOT NULL,
 online_checks INTEGER NOT NULL,
 telnet_only_checks INTEGER NOT NULL,
 tcp_only_checks INTEGER NOT NULL,
 offline_checks INTEGER NOT NULL,
 dns_fail_checks INTEGER NOT NULL,
 connect_ms_sum INTEGER NOT NULL,
 connect_ms_count INTEGER NOT NULL,
 PRIMARY KEY(endpoint_id,bucket_start)
);
CREATE TABLE IF NOT EXISTS public_dimension_snapshot (
 dimension TEXT NOT NULL,
 value TEXT NOT NULL,
 count INTEGER NOT NULL,
 refreshed_at TEXT NOT NULL,
 PRIMARY KEY(dimension,value)
);
CREATE TABLE IF NOT EXISTS public_bbs_presence (
 bbs_id INTEGER PRIMARY KEY REFERENCES bbs(id) ON DELETE CASCADE,
 active INTEGER NOT NULL,
 last_changed TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS public_lifecycle_daily (
 day TEXT PRIMARY KEY,
 new_bbs INTEGER NOT NULL DEFAULT 0,
 disappeared_bbs INTEGER NOT NULL DEFAULT 0,
 returned_bbs INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_probe_endpoint_checked ON probe_result(endpoint_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_probe_checked ON probe_result(checked_at);
CREATE INDEX IF NOT EXISTS idx_change_event_occurred ON change_event(occurred_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_change_event_bbs ON change_event(bbs_id,occurred_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_endpoint_hourly_bucket ON endpoint_hourly(bucket_start,endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_daily_bucket ON endpoint_daily(bucket_start,endpoint_id);
CREATE INDEX IF NOT EXISTS idx_public_dimension ON public_dimension_snapshot(dimension,count DESC,value);
`
	if _, err := s.DB.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE probe_result ADD COLUMN banner_sha256 TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE probe_result ADD COLUMN banner_preview TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE probe_result ADD COLUMN detected_software TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE probe_result ADD COLUMN software_confidence REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE probe_result ADD COLUMN software_evidence TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_entry ADD COLUMN reported_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_entry ADD COLUMN reported_software TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_entry ADD COLUMN reported_country TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_entry ADD COLUMN reported_description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_entry ADD COLUMN active INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE source_entry ADD COLUMN missing_since TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE source_health ADD COLUMN current_state TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE source_health ADD COLUMN state_changed_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil && !isDuplicateColumn(err) {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if _, err := s.DB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_source_entry_presence ON source_entry(source,active,last_seen)`); err != nil {
		return fmt.Errorf("migrate source presence index: %w", err)
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "duplicate column name")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
