package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/publicstats"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestMetricsHandlerAndHTTPObservation(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO bbs(id,name,software,country) VALUES(1,'Alpha','Synchronet','US')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO endpoint(id,bbs_id,protocol,hostname,port) VALUES(1,1,'telnet','alpha.test',23)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,checked_at,status,connect_ms,detected_software) VALUES(1,'2026-09-09 12:00:00','online',20,'Synchronet')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO source_health(source,last_attempt_at,last_success_at,last_entry_count,consecutive_failures,current_state) VALUES('testsource','2026-09-09 12:00:00','2026-09-09 12:00:00',1,0,'fresh')`); err != nil {
		t.Fatal(err)
	}
	if err := publicstats.RefreshRuntime(ctx, s.DB, time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	srv := &server{db: s.DB, metrics: newHTTPMetrics()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ping/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", srv.metricsHandler)
	h := srv.observeHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping/123?ignored=yes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("ping status=%d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("unexpected content type %q", got)
	}

	body := rr.Body.String()
	for _, want := range []string{
		"bbsintel_build_info",
		"bbsintel_database_up 1",
		"bbsintel_bbs 1",
		"bbsintel_endpoints 1",
		"bbsintel_probes_retained 1",
		"bbsintel_endpoint_status{status=\"online\"} 1",
		"bbsintel_source_entry_count{source=\"testsource\"} 1",
		"bbsintel_source_state_info{source=\"testsource\",state=\"fresh\"} 1",
		"route=\"GET /api/v1/ping/{id}\"",
		"status=\"204\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "/api/v1/ping/123") || strings.Contains(body, "ignored=yes") {
		t.Fatalf("metrics leaked raw request path/query into labels:\n%s", body)
	}
	if strings.Contains(body, `route="GET /metrics"`) {
		t.Fatalf("metrics scrape should not self-instrument:\n%s", body)
	}
}
