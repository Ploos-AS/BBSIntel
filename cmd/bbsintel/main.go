package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/buildinfo"
	"github.com/Ploos-AS/BBSIntel/internal/ingest"
	"github.com/Ploos-AS/BBSIntel/internal/probe"
	"github.com/Ploos-AS/BBSIntel/internal/scheduler"
	"github.com/Ploos-AS/BBSIntel/internal/source"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

type server struct {
	db *sql.DB
}

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-version") {
		i := buildinfo.Current()
		fmt.Printf("BBSIntel %s commit=%s date=%s\n", i.Version, i.Commit, i.Date)
		return
	}

	listen := env("BBSINTEL_LISTEN", ":8080")
	dbPath := env("BBSINTEL_DB", "./data/bbsintel.db")
	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	if len(os.Args) >= 3 && os.Args[1] == "import" {
		var adapter source.Adapter
		switch os.Args[2] {
		case "telnetbbsguide":
			adapter = &source.TelnetBBSGuide{}
		case "synchronet":
			adapter = &source.Synchronet{}
		default:
			log.Fatalf("unknown import source %q", os.Args[2])
		}
		n, err := ingest.Import(context.Background(), s.DB, adapter)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("imported %d entries from %s", n, adapter.Name())
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "probe" {
		concurrency := envInt("BBSINTEL_PROBE_CONCURRENCY", 8)
		baseInterval := envDuration("BBSINTEL_PROBE_INTERVAL", 30*time.Minute)
		n, err := (probe.Worker{DB: s.DB, Concurrency: concurrency, BaseInterval: baseInterval}).Run(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("probed %d endpoints", n)
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "maintenance" {
		runMaintenance(s)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	schedulerConfig := scheduler.Config{
		ImportInterval: envDuration("BBSINTEL_IMPORT_INTERVAL", 24*time.Hour),
		ProbeInterval:  envDuration("BBSINTEL_PROBE_INTERVAL", 30*time.Minute),
		Concurrency:    envInt("BBSINTEL_PROBE_CONCURRENCY", 8),
	}

	if len(os.Args) >= 2 && os.Args[1] == "scheduler" {
		err := (scheduler.Scheduler{DB: s.DB, Config: schedulerConfig}).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}

	if envBool("BBSINTEL_SCHEDULER_ENABLED", true) {
		go func() {
			err := (scheduler.Scheduler{DB: s.DB, Config: schedulerConfig}).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("scheduler stopped: %v", err)
			}
		}()
	}

	srv := &server{db: s.DB}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", srv.ready)
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, buildinfo.Current())
	})
	mux.HandleFunc("GET /api/v1/bbs", srv.listBBS)
	mux.HandleFunc("GET /api/v1/bbs/{id}", srv.getBBS)
	mux.HandleFunc("GET /api/v1/stats", srv.stats)
	registerAnalyticsRoutes(mux, srv)
	registerSourceRoutes(mux, srv)
	registerIntelRoutes(mux, srv)
	registerEventRoutes(mux, srv)
	registerAlertRoutes(mux, srv)

	httpServer := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: envDuration("BBSINTEL_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       envDuration("BBSINTEL_HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      envDuration("BBSINTEL_HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:       envDuration("BBSINTEL_HTTP_IDLE_TIMEOUT", 60*time.Second),
		MaxHeaderBytes:    envInt("BBSINTEL_HTTP_MAX_HEADER_BYTES", 1<<20),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("BBSIntel %s listening on %s", buildinfo.Version, listen)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (s *server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	var one int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	jsonOut(w, map[string]string{"status": "ready"})
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func envInt(k string, fallback int) int {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envDuration(k string, fallback time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func envBool(k string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func softwareMismatch(reported, observed string) bool {
	if strings.TrimSpace(reported) == "" || strings.TrimSpace(observed) == "" {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(reported), strings.TrimSpace(observed))
}
