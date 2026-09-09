package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/publicstats"
)

func registerPublicStatisticsRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/statistics/dimensions/{dimension}", srv.publicDimensionStatistics)
	mux.HandleFunc("GET /api/v1/statistics/lifecycle", srv.publicLifecycleStatistics)
}

func (s *server) publicDimensionStatistics(w http.ResponseWriter, r *http.Request) {
	dimension := strings.ToLower(strings.TrimSpace(r.PathValue("dimension")))
	switch dimension {
	case "software", "protocol", "country", "source":
	default:
		http.Error(w, "dimension must be one of software, protocol, country, source", http.StatusBadRequest)
		return
	}
	rows, err := publicstats.Dimensions(r.Context(), s.db, dimension)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	jsonOut(w, rows)
}

func (s *server) publicLifecycleStatistics(w http.ResponseWriter, r *http.Request) {
	days := 365
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 3650 {
			http.Error(w, "days must be between 1 and 3650", http.StatusBadRequest)
			return
		}
		days = n
	}
	rows, err := publicstats.Lifecycle(r.Context(), s.db, days, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	jsonOut(w, rows)
}
