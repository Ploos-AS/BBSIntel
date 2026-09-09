package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/rollup"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func runMaintenance(s *store.Store) {
	if s == nil || s.DB == nil {
		log.Fatal("maintenance: database unavailable")
	}
	if len(os.Args) < 3 {
		log.Fatal("usage: bbsintel maintenance <rollup|prune|all>")
	}

	ctx := context.Background()
	now := time.Now().UTC()
	switch os.Args[2] {
	case "rollup":
		if err := rollup.Refresh(ctx, s.DB, now); err != nil {
			log.Fatal(err)
		}
		log.Printf("maintenance rollup complete")
	case "prune":
		pruneRaw(ctx, s, now)
	case "all":
		if err := rollup.Refresh(ctx, s.DB, now); err != nil {
			log.Fatal(err)
		}
		log.Printf("maintenance rollup complete")
		pruneRaw(ctx, s, now)
	default:
		log.Fatalf("unknown maintenance command %q", os.Args[2])
	}
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
