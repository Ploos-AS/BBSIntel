package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/dbmaint"
	"github.com/Ploos-AS/BBSIntel/internal/publicstats"
	"github.com/Ploos-AS/BBSIntel/internal/rollup"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func runMaintenance(s *store.Store) {
	if s == nil || s.DB == nil {
		log.Fatal("maintenance: database unavailable")
	}
	if len(os.Args) < 3 {
		log.Fatal("usage: bbsintel maintenance <rollup|prune|all|check|backup|restore>")
	}

	ctx := context.Background()
	now := time.Now().UTC()
	switch os.Args[2] {
	case "rollup":
		refreshStatistics(ctx, s, now)
	case "prune":
		pruneRaw(ctx, s, now)
	case "all":
		refreshStatistics(ctx, s, now)
		pruneRaw(ctx, s, now)
	case "check":
		full := false
		if len(os.Args) >= 4 {
			switch strings.ToLower(strings.TrimSpace(os.Args[3])) {
			case "quick":
			case "full":
				full = true
			default:
				log.Fatal("usage: bbsintel maintenance check [quick|full]")
			}
		}
		if err := dbmaint.Check(ctx, s.DB, full); err != nil {
			log.Fatal(err)
		}
		mode := "quick_check"
		if full {
			mode = "integrity_check"
		}
		log.Printf("maintenance database check complete: %s=ok", mode)
	case "backup":
		destination := ""
		if len(os.Args) >= 4 {
			destination = strings.TrimSpace(os.Args[3])
		}
		if destination == "" {
			dbPath := env("BBSINTEL_DB", "./data/bbsintel.db")
			destination = filepath.Join(filepath.Dir(dbPath), "backups", "bbsintel-"+now.Format("20060102T150405Z")+".db")
		}
		if err := dbmaint.Backup(ctx, s.DB, destination); err != nil {
			log.Fatal(err)
		}
		if err := dbmaint.ValidateFile(ctx, destination); err != nil {
			_ = os.Remove(destination)
			log.Fatalf("backup verification failed: %v", err)
		}
		log.Printf("maintenance backup complete: %s", destination)
	case "restore":
		log.Fatal("restore must run before the database is opened; use: bbsintel maintenance restore <backup.db>")
	default:
		log.Fatalf("unknown maintenance command %q", os.Args[2])
	}
}

func refreshStatistics(ctx context.Context, s *store.Store, now time.Time) {
	if err := rollup.Refresh(ctx, s.DB, now); err != nil {
		log.Fatal(err)
	}
	if err := publicstats.Refresh(ctx, s.DB, now); err != nil {
		log.Fatal(err)
	}
	if err := publicstats.RefreshRuntime(ctx, s.DB, now); err != nil {
		log.Fatal(err)
	}
	log.Printf("maintenance statistics refresh complete")
}

func pruneRaw(ctx context.Context, s *store.Store, now time.Time) {
	retention := envDuration("BBSINTEL_RAW_RETENTION", 90*24*time.Hour)
	deleted, err := rollup.PruneRaw(ctx, s.DB, now, retention)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("maintenance prune complete: retention=%s rows_deleted=%d", fmtDuration(retention), deleted)
}

func fmtDuration(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int64(d/(24*time.Hour)))
	}
	return d.String()
}
