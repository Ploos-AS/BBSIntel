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
	mux.HandleFunc("GET /api/v1/statistics/daily", srv.dailyStatistics)
	registerPublicStatisticsRoutes(mux, srv)
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

type dailyStatistic struct {
	Date             string  `json:"date"`
	Checks           int64   `json:"checks"`
	OnlineChecks     int64   `json:"online_checks"`
	ConnectedChecks  int64   `json:"connected_checks"`
	OfflineChecks    int64   `json:"offline_checks"`
	DNSFailChecks    int64   `json:"dns_fail_checks"`
	OnlinePct        float64 `json:"online_pct"`
	ConnectedPct     float64 `json:"connected_pct"`
	AverageConnectMS float64 `json:"average_connect_ms"`
}

func (s *server) dailyStatistics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 3650 {
			http.Error(w, "days must be between 1 and 3650", http.StatusBadRequest)
			return
		}
		days = n
	}

	rows, err := s.db.QueryContext(r.Context(), `
SELECT substr(bucket_start,1,10),
       SUM(checks),SUM(online_checks),SUM(online_checks+telnet_only_checks+tcp_only_checks),
       SUM(offline_checks),SUM(dns_fail_checks),SUM(connect_ms_sum),SUM(connect_ms_count)
FROM endpoint_daily
WHERE bucket_start>=?
GROUP BY substr(bucket_start,1,10)
ORDER BY bucket_start`, time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02 00:00:00"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]dailyStatistic, 0, days)
	for rows.Next() {
		var d dailyStatistic
		var connectSum, connectCount int64
		if err := rows.Scan(&d.Date, &d.Checks, &d.OnlineChecks, &d.ConnectedChecks, &d.OfflineChecks, &d.DNSFailChecks, &connectSum, &connectCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if d.Checks > 0 {
			d.OnlinePct = float64(d.OnlineChecks) * 100 / float64(d.Checks)
			d.ConnectedPct = float64(d.ConnectedChecks) * 100 / float64(d.Checks)
		}
		if connectCount > 0 {
			d.AverageConnectMS = float64(connectSum) / float64(connectCount)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	jsonOut(w, out)
}
