package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/alerts"
)

func registerAlertRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/alerts", srv.listAlerts)
}

func (s *server) listAlerts(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	minimumSeverity := strings.TrimSpace(r.URL.Query().Get("severity"))
	out, err := alerts.List(r.Context(), s.db, limit, minimumSeverity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, out)
}
