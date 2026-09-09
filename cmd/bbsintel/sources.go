package main

import (
	"database/sql"
	"net/http"
	"strings"
)

type sourceView struct {
	Source              string `json:"source"`
	SourceKey           string `json:"source_key"`
	SourceURL           string `json:"source_url"`
	ReportedName        string `json:"reported_name"`
	ReportedSoftware    string `json:"reported_software"`
	ReportedCountry     string `json:"reported_country"`
	ReportedDescription string `json:"reported_description"`
	LastSeen            string `json:"last_seen"`
	Active              bool   `json:"active"`
	MissingSince        string `json:"missing_since,omitempty"`
}

type sourceHealthView struct {
	Source              string `json:"source"`
	LastAttemptAt       string `json:"last_attempt_at"`
	LastSuccessAt       string `json:"last_success_at"`
	LastFailureAt       string `json:"last_failure_at"`
	LastDurationMS      int64  `json:"last_duration_ms"`
	LastEntryCount      int    `json:"last_entry_count"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastError           string `json:"last_error,omitempty"`
	Healthy             bool   `json:"healthy"`
}

func registerSourceRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/bbs/{id}/sources", srv.getBBSSources)
	mux.HandleFunc("GET /api/v1/sources/health", srv.getSourceHealth)
	registerIntelRoutes(mux, srv)
}

func (s *server) getBBSSources(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM bbs WHERE id=?`, id).Scan(&exists); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
SELECT source,source_key,source_url,reported_name,reported_software,reported_country,reported_description,last_seen,active,missing_since
FROM source_entry WHERE bbs_id=? ORDER BY active DESC,source,source_key`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []sourceView{}
	for rows.Next() {
		var v sourceView
		var active int
		if err := rows.Scan(&v.Source, &v.SourceKey, &v.SourceURL, &v.ReportedName, &v.ReportedSoftware, &v.ReportedCountry, &v.ReportedDescription, &v.LastSeen, &active, &v.MissingSince); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		v.Active = active != 0
		out = append(out, v)
	}
	jsonOut(w, out)
}

func (s *server) getSourceHealth(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
SELECT source,last_attempt_at,last_success_at,last_failure_at,last_duration_ms,last_entry_count,consecutive_failures,last_error
FROM source_health ORDER BY source`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []sourceHealthView{}
	for rows.Next() {
		var v sourceHealthView
		if err := rows.Scan(&v.Source, &v.LastAttemptAt, &v.LastSuccessAt, &v.LastFailureAt, &v.LastDurationMS, &v.LastEntryCount, &v.ConsecutiveFailures, &v.LastError); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		v.Healthy = v.LastSuccessAt != "" && v.ConsecutiveFailures == 0
		out = append(out, v)
	}
	jsonOut(w, out)
}
