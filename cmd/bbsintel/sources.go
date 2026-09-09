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
}

func registerSourceRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/bbs/{id}/sources", srv.getBBSSources)
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
SELECT source,source_key,source_url,reported_name,reported_software,reported_country,reported_description,last_seen
FROM source_entry WHERE bbs_id=? ORDER BY source,source_key`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []sourceView{}
	for rows.Next() {
		var v sourceView
		if err := rows.Scan(&v.Source, &v.SourceKey, &v.SourceURL, &v.ReportedName, &v.ReportedSoftware, &v.ReportedCountry, &v.ReportedDescription, &v.LastSeen); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, v)
	}
	jsonOut(w, out)
}
