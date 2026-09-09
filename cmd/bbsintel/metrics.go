package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/buildinfo"
	"github.com/Ploos-AS/BBSIntel/internal/publicstats"
)

type httpMetricKey struct {
	Method string
	Route  string
	Status int
}

type httpMetricValue struct {
	Requests uint64
	Seconds  float64
}

type httpMetrics struct {
	mu     sync.RWMutex
	values map[httpMetricKey]httpMetricValue
}

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{values: make(map[httpMetricKey]httpMetricValue)}
}

func (m *httpMetrics) observe(method, route string, status int, elapsed time.Duration) {
	if m == nil || route == "GET /metrics" {
		return
	}
	if route == "" {
		route = "unmatched"
	}
	key := httpMetricKey{Method: method, Route: route, Status: status}
	m.mu.Lock()
	v := m.values[key]
	v.Requests++
	v.Seconds += elapsed.Seconds()
	m.values[key] = v
	m.mu.Unlock()
}

type metricResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *metricResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (s *server) observeHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &metricResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		status := rw.status
		if status == 0 {
			status = http.StatusOK
		}
		s.metrics.observe(r.Method, r.Pattern, status, time.Since(start))
	})
}

func promLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

func parseMetricTime(v string) (time.Time, bool) {
	if strings.TrimSpace(v) == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, time.RFC3339Nano} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func (s *server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	info := buildinfo.Current()
	fmt.Fprintf(w, "# HELP bbsintel_build_info Build metadata.\n# TYPE bbsintel_build_info gauge\n")
	fmt.Fprintf(w, "bbsintel_build_info{version=\"%s\",commit=\"%s\",date=\"%s\"} 1\n", promLabel(info.Version), promLabel(info.Commit), promLabel(info.Date))

	dbUp := 1
	if err := s.db.PingContext(r.Context()); err != nil {
		dbUp = 0
	}
	fmt.Fprintf(w, "# HELP bbsintel_database_up Whether SQLite is reachable.\n# TYPE bbsintel_database_up gauge\nbbsintel_database_up %d\n", dbUp)

	if snapshot, err := publicstats.Runtime(r.Context(), s.db); err == nil {
		fmt.Fprintln(w, "# HELP bbsintel_bbs Current BBS count from the materialized runtime snapshot.")
		fmt.Fprintln(w, "# TYPE bbsintel_bbs gauge")
		fmt.Fprintf(w, "bbsintel_bbs %d\n", snapshot.BBS)
		fmt.Fprintln(w, "# HELP bbsintel_endpoints Current endpoint count from the materialized runtime snapshot.")
		fmt.Fprintln(w, "# TYPE bbsintel_endpoints gauge")
		fmt.Fprintf(w, "bbsintel_endpoints %d\n", snapshot.Endpoints)
		fmt.Fprintln(w, "# HELP bbsintel_probes_retained Raw probe rows currently retained.")
		fmt.Fprintln(w, "# TYPE bbsintel_probes_retained gauge")
		fmt.Fprintf(w, "bbsintel_probes_retained %d\n", snapshot.Probes)
		fmt.Fprintln(w, "# HELP bbsintel_software_mismatches Current reported/observed software mismatches.")
		fmt.Fprintln(w, "# TYPE bbsintel_software_mismatches gauge")
		fmt.Fprintf(w, "bbsintel_software_mismatches %d\n", snapshot.SoftwareMismatches)
		fmt.Fprintln(w, "# HELP bbsintel_endpoint_status Current endpoint status counts.")
		fmt.Fprintln(w, "# TYPE bbsintel_endpoint_status gauge")
		statuses := make([]string, 0, len(snapshot.Status))
		for status := range snapshot.Status {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			fmt.Fprintf(w, "bbsintel_endpoint_status{status=\"%s\"} %d\n", promLabel(status), snapshot.Status[status])
		}
		if refreshed, ok := parseMetricTime(snapshot.RefreshedAt); ok {
			age := time.Since(refreshed).Seconds()
			if age < 0 {
				age = 0
			}
			fmt.Fprintln(w, "# HELP bbsintel_runtime_snapshot_age_seconds Age of the materialized runtime snapshot.")
			fmt.Fprintln(w, "# TYPE bbsintel_runtime_snapshot_age_seconds gauge")
			fmt.Fprintf(w, "bbsintel_runtime_snapshot_age_seconds %.3f\n", age)
			fmt.Fprintln(w, "# HELP bbsintel_runtime_snapshot_timestamp_seconds Runtime snapshot refresh time.")
			fmt.Fprintln(w, "# TYPE bbsintel_runtime_snapshot_timestamp_seconds gauge")
			fmt.Fprintf(w, "bbsintel_runtime_snapshot_timestamp_seconds %d\n", refreshed.Unix())
		}
	}

	s.writeSourceMetrics(w, r)
	s.writeHTTPMetrics(w)
}

func (s *server) writeSourceMetrics(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
SELECT source,last_attempt_at,last_success_at,last_entry_count,consecutive_failures,current_state
FROM source_health ORDER BY source`)
	if err != nil {
		return
	}
	defer rows.Close()

	fmt.Fprintln(w, "# HELP bbsintel_source_last_attempt_timestamp_seconds Last source import attempt time.")
	fmt.Fprintln(w, "# TYPE bbsintel_source_last_attempt_timestamp_seconds gauge")
	fmt.Fprintln(w, "# HELP bbsintel_source_last_success_timestamp_seconds Last successful source import time.")
	fmt.Fprintln(w, "# TYPE bbsintel_source_last_success_timestamp_seconds gauge")
	fmt.Fprintln(w, "# HELP bbsintel_source_entry_count Entries seen during the last successful source import.")
	fmt.Fprintln(w, "# TYPE bbsintel_source_entry_count gauge")
	fmt.Fprintln(w, "# HELP bbsintel_source_consecutive_failures Consecutive source import failures.")
	fmt.Fprintln(w, "# TYPE bbsintel_source_consecutive_failures gauge")
	fmt.Fprintln(w, "# HELP bbsintel_source_state_info Current source health state.")
	fmt.Fprintln(w, "# TYPE bbsintel_source_state_info gauge")

	for rows.Next() {
		var source, attempt, success, state string
		var entries, failures int64
		if err := rows.Scan(&source, &attempt, &success, &entries, &failures, &state); err != nil {
			return
		}
		label := promLabel(source)
		if t, ok := parseMetricTime(attempt); ok {
			fmt.Fprintf(w, "bbsintel_source_last_attempt_timestamp_seconds{source=\"%s\"} %d\n", label, t.Unix())
		}
		if t, ok := parseMetricTime(success); ok {
			fmt.Fprintf(w, "bbsintel_source_last_success_timestamp_seconds{source=\"%s\"} %d\n", label, t.Unix())
		}
		fmt.Fprintf(w, "bbsintel_source_entry_count{source=\"%s\"} %d\n", label, entries)
		fmt.Fprintf(w, "bbsintel_source_consecutive_failures{source=\"%s\"} %d\n", label, failures)
		fmt.Fprintf(w, "bbsintel_source_state_info{source=\"%s\",state=\"%s\"} 1\n", label, promLabel(state))
	}
}

func (s *server) writeHTTPMetrics(w http.ResponseWriter) {
	if s.metrics == nil {
		return
	}
	s.metrics.mu.RLock()
	keys := make([]httpMetricKey, 0, len(s.metrics.values))
	for key := range s.metrics.values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Route != keys[j].Route {
			return keys[i].Route < keys[j].Route
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})
	values := make(map[httpMetricKey]httpMetricValue, len(keys))
	for _, key := range keys {
		values[key] = s.metrics.values[key]
	}
	s.metrics.mu.RUnlock()

	fmt.Fprintln(w, "# HELP bbsintel_http_requests_total HTTP requests by method, route pattern, and status.")
	fmt.Fprintln(w, "# TYPE bbsintel_http_requests_total counter")
	fmt.Fprintln(w, "# HELP bbsintel_http_request_duration_seconds HTTP request duration summary by method, route pattern, and status.")
	fmt.Fprintln(w, "# TYPE bbsintel_http_request_duration_seconds summary")
	for _, key := range keys {
		v := values[key]
		labels := fmt.Sprintf("method=\"%s\",route=\"%s\",status=\"%s\"", promLabel(key.Method), promLabel(key.Route), strconv.Itoa(key.Status))
		fmt.Fprintf(w, "bbsintel_http_requests_total{%s} %d\n", labels, v.Requests)
		fmt.Fprintf(w, "bbsintel_http_request_duration_seconds_sum{%s} %.6f\n", labels, v.Seconds)
		fmt.Fprintf(w, "bbsintel_http_request_duration_seconds_count{%s} %d\n", labels, v.Requests)
	}
}
