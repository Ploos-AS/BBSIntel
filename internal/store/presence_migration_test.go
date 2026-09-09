package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/store"
	_ "modernc.org/sqlite"
)

func TestPresenceColumnsMigrateFromOlderSourceEntrySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bbsintel.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE bbs (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL,
 software TEXT NOT NULL DEFAULT '',
 country TEXT NOT NULL DEFAULT '',
 description TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE source_entry (
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
INSERT INTO bbs(name) VALUES('Legacy BBS');
INSERT INTO source_entry(bbs_id,source,source_key) VALUES(1,'legacy','legacy-key');
`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var active int
	var missingSince string
	if err := s.DB.QueryRow(`SELECT active,missing_since FROM source_entry WHERE source='legacy'`).Scan(&active, &missingSince); err != nil {
		t.Fatal(err)
	}
	if active != 1 || missingSince != "" {
		t.Fatalf("migrated presence active=%d missing_since=%q, want 1 and empty", active, missingSince)
	}
}
