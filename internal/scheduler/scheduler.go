package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/ingest"
	"github.com/Ploos-AS/BBSIntel/internal/probe"
	"github.com/Ploos-AS/BBSIntel/internal/source"
)

type Config struct {
	ImportInterval time.Duration
	ProbeInterval  time.Duration
	Concurrency    int
}

type Scheduler struct {
	DB     *sql.DB
	Config Config
}

func (s Scheduler) Run(ctx context.Context) error {
	cfg := s.Config
	if cfg.ImportInterval <= 0 {
		cfg.ImportInterval = 24 * time.Hour
	}
	if cfg.ProbeInterval <= 0 {
		cfg.ProbeInterval = 30 * time.Minute
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 8
	}

	if err := s.importOnce(ctx); err != nil {
		log.Printf("initial import failed: %v", err)
	}
	if err := s.probeOnce(ctx, cfg.Concurrency, cfg.ProbeInterval); err != nil {
		log.Printf("initial probe failed: %v", err)
	}

	importTicker := time.NewTicker(cfg.ImportInterval)
	defer importTicker.Stop()
	probeTicker := time.NewTicker(cfg.ProbeInterval)
	defer probeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-importTicker.C:
			if err := s.importOnce(ctx); err != nil {
				log.Printf("scheduled import failed: %v", err)
			}
		case <-probeTicker.C:
			if err := s.probeOnce(ctx, cfg.Concurrency, cfg.ProbeInterval); err != nil {
				log.Printf("scheduled probe failed: %v", err)
			}
		}
	}
}

func (s Scheduler) importOnce(ctx context.Context) error {
	adapters := []source.Adapter{
		&source.TelnetBBSGuide{},
		&source.Synchronet{},
	}
	var errs []error
	for _, adapter := range adapters {
		n, err := ingest.Import(ctx, s.DB, adapter)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", adapter.Name(), err))
			continue
		}
		log.Printf("scheduled import completed: source=%s entries=%d", adapter.Name(), n)
	}
	return errors.Join(errs...)
}

func (s Scheduler) probeOnce(ctx context.Context, concurrency int, baseInterval time.Duration) error {
	n, err := (probe.Worker{DB: s.DB, Concurrency: concurrency, BaseInterval: baseInterval}).Run(ctx)
	if err != nil {
		return err
	}
	log.Printf("scheduled probe completed: %d endpoints probed", n)
	return nil
}
