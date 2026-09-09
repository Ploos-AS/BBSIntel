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
 UNIQUE(source, source_key)
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
CREATE INDEX IF NOT EXISTS idx_probe_endpoint_checked ON probe_result(endpoint_id, checked_at DESC);
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
	} {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil && !isDuplicateColumn(err) {
			return fmt.Errorf("migrate: %w", err)
		}
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
