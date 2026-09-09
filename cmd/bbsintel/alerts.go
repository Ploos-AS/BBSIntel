package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/alerts"
)

func registerAlertRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/alerts", srv.listAlerts)
	mux.HandleFunc("GET /api/v1/alerts/stats", srv.alertStats)
}

func alertFilter(r *http.Request) alerts.Filter {
	filter := alerts.Filter{
		Limit:           200,
		MinimumSeverity: strings.TrimSpace(r.URL.Query().Get("severity")),
		Category:        strings.TrimSpace(r.URL.Query().Get("category")),
		Source:          strings.TrimSpace(r.URL.Query().Get("source")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("bbs_id")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			filter.BBSID = n
		}
	}
	return filter
}

func (s *server) listAlerts(w http.ResponseWriter, r *http.Request) {
	out, err := alerts.Query(r.Context(), s.db, alertFilter(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, out)
}

func (s *server) alertStats(w http.ResponseWriter, r *http.Request) {
	filter := alertFilter(r)
	filter.Limit = 0
	out, err := alerts.Summarize(r.Context(), s.db, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, out)
}
