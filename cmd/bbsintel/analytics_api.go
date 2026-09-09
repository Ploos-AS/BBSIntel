package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/analytics"
)

func registerAnalyticsRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/bbs/{id}/analytics", srv.bbsAnalytics)
	mux.HandleFunc("GET /api/v1/bbs/{id}/history", srv.bbsHistory)
}

func (s *server) bbsAnalytics(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	summary, err := analytics.ForBBS(r.Context(), s.db, id, time.Now().UTC())
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, summary)
}

func (s *server) bbsHistory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM bbs WHERE id=?`, id).Scan(&exists); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	history, err := analytics.History(r.Context(), s.db, id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, history)
}
