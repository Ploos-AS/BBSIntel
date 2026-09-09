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

	httpServer := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
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

func (s *server) listBBS(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
SELECT b.id,b.name,b.software,b.country,
       COALESCE(e.protocol,''),COALESCE(e.hostname,''),COALESCE(e.port,0),
       COALESCE(pr.status,''),COALESCE(pr.checked_at,''),COALESCE(pr.connect_ms,0),
       COALESCE(pr.detected_software,''),COALESCE(pr.software_confidence,0),COALESCE(pr.software_evidence,'')
FROM bbs b
LEFT JOIN endpoint e ON e.id=(SELECT e2.id FROM endpoint e2 WHERE e2.bbs_id=b.id ORDER BY e2.id LIMIT 1)
LEFT JOIN probe_result pr ON pr.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
ORDER BY lower(b.name)`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, reportedSoftware, country, protocol, hostname, status, checkedAt, observedSoftware, evidence string
		var port int
		var connectMS int64
		var confidence float64
		if err := rows.Scan(&id, &name, &reportedSoftware, &country, &protocol, &hostname, &port, &status, &checkedAt, &connectMS, &observedSoftware, &confidence, &evidence); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "reported_software": reportedSoftware, "observed_software": observedSoftware,
			"software_confidence": confidence, "software_evidence": evidence,
			"software_mismatch": softwareMismatch(reportedSoftware, observedSoftware), "country": country,
			"protocol": protocol, "hostname": hostname, "port": port,
			"status": status, "checked_at": checkedAt, "connect_ms": connectMS,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, out)
}

func (s *server) getBBS(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var name, reportedSoftware, country, description string
	err := s.db.QueryRowContext(r.Context(), `SELECT name,software,country,description FROM bbs WHERE id=?`, id).Scan(&name, &reportedSoftware, &country, &description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
SELECT e.id,e.protocol,e.hostname,e.port,
       COALESCE(pr.status,''),COALESCE(pr.checked_at,''),COALESCE(pr.connect_ms,0),COALESCE(pr.banner_bytes,0),
       COALESCE(pr.banner_sha256,''),COALESCE(pr.banner_preview,''),COALESCE(pr.detected_software,''),
       COALESCE(pr.software_confidence,0),COALESCE(pr.software_evidence,''),COALESCE(pr.error,'')
FROM endpoint e
LEFT JOIN probe_result pr ON pr.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
WHERE e.bbs_id=? ORDER BY e.id`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	endpoints := []map[string]any{}
	observedSoftware := ""
	maxConfidence := 0.0
	for rows.Next() {
		var endpointID int64
		var protocol, hostname, status, checkedAt, bannerSHA, bannerPreview, detectedSoftware, evidence, probeError string
		var port, bannerBytes int
		var connectMS int64
		var confidence float64
		if err := rows.Scan(&endpointID, &protocol, &hostname, &port, &status, &checkedAt, &connectMS, &bannerBytes, &bannerSHA, &bannerPreview, &detectedSoftware, &confidence, &evidence, &probeError); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if confidence > maxConfidence && detectedSoftware != "" {
			observedSoftware = detectedSoftware
			maxConfidence = confidence
		}
		endpoints = append(endpoints, map[string]any{
			"id": endpointID, "protocol": protocol, "hostname": hostname, "port": port,
			"status": status, "checked_at": checkedAt, "connect_ms": connectMS,
			"banner_bytes": bannerBytes, "banner_sha256": bannerSHA, "banner_preview": bannerPreview,
			"observed_software": detectedSoftware, "software_confidence": confidence,
			"software_evidence": evidence, "error": probeError,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{
		"id": id, "name": name, "reported_software": reportedSoftware, "observed_software": observedSoftware,
		"software_confidence": maxConfidence, "software_mismatch": softwareMismatch(reportedSoftware, observedSoftware),
		"country": country, "description": description, "endpoints": endpoints,
	})
}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	var bbs, endpoints, probes int64
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bbs`).Scan(&bbs)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM endpoint`).Scan(&endpoints)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM probe_result`).Scan(&probes)

	statusCounts := map[string]int64{}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT status,count(*) FROM probe_result p
WHERE p.id IN (SELECT (SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1) FROM endpoint e)
GROUP BY status`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			statusCounts[status] = count
		}
		if err := rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	var mismatches int64
	_ = s.db.QueryRowContext(r.Context(), `
SELECT count(*) FROM bbs b
WHERE trim(b.software)<>'' AND EXISTS (
 SELECT 1 FROM endpoint e JOIN probe_result p ON p.endpoint_id=e.id
 WHERE e.bbs_id=b.id AND trim(p.detected_software)<>'' AND lower(trim(p.detected_software))<>lower(trim(b.software))
 AND p.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
)`).Scan(&mismatches)

	jsonOut(w, map[string]any{"bbs": bbs, "endpoints": endpoints, "probes": probes, "status": statusCounts, "software_mismatches": mismatches})
}
