package dbmaint

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Check runs SQLite's quick_check or integrity_check and returns an error when
// SQLite reports anything other than "ok".
func Check(ctx context.Context, db *sql.DB, full bool) error {
	if db == nil {
		return fmt.Errorf("database unavailable")
	}
	pragma := "PRAGMA quick_check"
	if full {
		pragma = "PRAGMA integrity_check"
	}
	rows, err := db.QueryContext(ctx, pragma)
	if err != nil {
		return fmt.Errorf("%s: %w", pragma, err)
	}
	defer rows.Close()

	var problems []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if strings.TrimSpace(result) != "ok" {
			problems = append(problems, result)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(problems) != 0 {
		return fmt.Errorf("sqlite integrity check failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Backup creates a transactionally consistent standalone SQLite database using
// VACUUM INTO. The destination must not already exist.
func Backup(ctx context.Context, db *sql.DB, destination string) error {
	if db == nil {
		return fmt.Errorf("database unavailable")
	}
	destination = filepath.Clean(strings.TrimSpace(destination))
	if destination == "." || destination == "" {
		return fmt.Errorf("backup destination is required")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	// VACUUM INTO accepts an SQL expression, so the filename can be bound.
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return fmt.Errorf("sqlite backup: %w", err)
	}
	return nil
}

// ValidateFile opens a backup read-only and checks its integrity without
// applying BBSIntel migrations.
func ValidateFile(ctx context.Context, path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return fmt.Errorf("backup path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup is not a regular file: %s", path)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	return Check(ctx, db, true)
}

// RestoreFile replaces destination with a validated backup. The caller must
// ensure no BBSIntel process has the destination database open. If destination
// exists it is first renamed to a timestamped pre-restore copy.
func RestoreFile(ctx context.Context, source, destination string, now time.Time) (string, error) {
	if err := ValidateFile(ctx, source); err != nil {
		return "", fmt.Errorf("backup validation failed: %w", err)
	}
	source = filepath.Clean(source)
	destination = filepath.Clean(destination)
	if source == destination {
		return "", fmt.Errorf("backup and destination must differ")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return "", err
	}

	previous := ""
	if _, err := os.Stat(destination); err == nil {
		previous = fmt.Sprintf("%s.pre-restore-%s", destination, now.UTC().Format("20060102T150405Z"))
		if _, err := os.Stat(previous); err == nil {
			return "", fmt.Errorf("pre-restore destination already exists: %s", previous)
		}
		if err := os.Rename(destination, previous); err != nil {
			return "", fmt.Errorf("preserve current database: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	in, err := os.Open(source)
	if err != nil {
		return previous, rollbackRestore(previous, destination, err)
	}
	defer in.Close()

	tmp := destination + ".restore-tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return previous, rollbackRestore(previous, destination, err)
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		err := copyErr
		if err == nil {
			err = syncErr
		}
		if err == nil {
			err = closeErr
		}
		return previous, rollbackRestore(previous, destination, err)
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return previous, rollbackRestore(previous, destination, err)
	}
	// The service must be offline during restore; remove stale WAL sidecars so
	// they can never be replayed against the restored main database.
	_ = os.Remove(destination + "-wal")
	_ = os.Remove(destination + "-shm")
	return previous, nil
}

func rollbackRestore(previous, destination string, cause error) error {
	if previous == "" {
		return cause
	}
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if restoreErr := os.Rename(previous, destination); restoreErr != nil {
			return fmt.Errorf("restore failed: %v; rollback failed: %w", cause, restoreErr)
		}
	}
	return cause
}
