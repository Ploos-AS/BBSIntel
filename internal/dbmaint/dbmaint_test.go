package dbmaint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bbsintel.db")
	backupPath := filepath.Join(dir, "backups", "snapshot.db")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO bbs(name,software,country) VALUES('Alpha BBS','Mystic','NO')`); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, s.DB, false); err != nil {
		t.Fatalf("quick check: %v", err)
	}
	if err := Backup(ctx, s.DB, backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := ValidateFile(ctx, backupPath); err != nil {
		t.Fatalf("validate backup: %v", err)
	}
	if _, err := s.DB.Exec(`UPDATE bbs SET name='Changed After Backup'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	previous, err := RestoreFile(ctx, backupPath, dbPath, now)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if previous == "" {
		t.Fatal("expected pre-restore database path")
	}
	if _, err := os.Stat(previous); err != nil {
		t.Fatalf("pre-restore database missing: %v", err)
	}

	restored, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name string
	if err := restored.DB.QueryRow(`SELECT name FROM bbs`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Alpha BBS" {
		t.Fatalf("restored name=%q, want Alpha BBS", name)
	}
	if err := Check(ctx, restored.DB, true); err != nil {
		t.Fatalf("restored integrity: %v", err)
	}
}

func TestBackupRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bbsintel.db")
	backupPath := filepath.Join(dir, "backup.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := os.WriteFile(backupPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Backup(context.Background(), s.DB, backupPath); err == nil {
		t.Fatal("expected existing backup destination to be rejected")
	}
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me" {
		t.Fatalf("existing backup was modified: %q", got)
	}
}
