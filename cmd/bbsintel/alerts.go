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

func alertFilter(r *http.Request) (alerts.Filter, error) {
	since, err := normalizeSince(r.URL.Query().Get("since"))
	if err != nil {
		return alerts.Filter{}, err
	}
	filter := alerts.Filter{
		Limit:           200,
		MinimumSeverity: strings.TrimSpace(r.URL.Query().Get("severity")),
		Category:        strings.TrimSpace(r.URL.Query().Get("category")),
		Source:          strings.TrimSpace(r.URL.Query().Get("source")),
		Since:           since,
		Cursor:          strings.TrimSpace(r.URL.Query().Get("cursor")),
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
	return filter, nil
}

func (s *server) listAlerts(w http.ResponseWriter, r *http.Request) {
	filter, err := alertFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	page, err := alerts.QueryPage(r.Context(), s.db, filter)
	if err != nil {
		if strings.Contains(err.Error(), "cursor") {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if page.NextCursor != "" {
		w.Header().Set("X-Next-Cursor", page.NextCursor)
	}
	jsonOut(w, page.Items)
}

func (s *server) alertStats(w http.ResponseWriter, r *http.Request) {
	filter, err := alertFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filter.Limit = 0
	filter.Cursor = ""
	out, err := alerts.Summarize(r.Context(), s.db, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, out)
}
