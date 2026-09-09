package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct { DB *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return nil, err }
	db, err := sql.Open("sqlite", path)
	if err != nil { return nil, err }
	s := &Store{DB: db}
	if err := s.migrate(context.Background()); err != nil { db.Close(); return nil, err }
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA foreign_keys=ON;
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
 error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_probe_endpoint_checked ON probe_result(endpoint_id, checked_at DESC);
`
	if _, err := s.DB.ExecContext(ctx, schema); err != nil { return fmt.Errorf("migrate: %w", err) }
	return nil
}
