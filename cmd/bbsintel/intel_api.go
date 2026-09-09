package main

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/intel"
)

func registerIntelRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/bbs/{id}/intelligence", srv.getBBSIntelligence)
	mux.HandleFunc("GET /api/v1/intelligence/stats", srv.getIntelligenceStats)
}

func (s *server) getBBSIntelligence(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM bbs WHERE id=?`, id).Scan(&exists); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	summary, err := intel.ForBBS(r.Context(), s.db, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, summary)
}

func (s *server) getIntelligenceStats(w http.ResponseWriter, r *http.Request) {
	queries := map[string]string{
		"multi_source_bbs": `SELECT count(*) FROM bbs b WHERE (SELECT count(*) FROM source_entry s WHERE s.bbs_id=b.id)>1`,
		"name_conflicts": `SELECT count(*) FROM bbs b WHERE (SELECT count(DISTINCT lower(trim(reported_name))) FROM source_entry s WHERE s.bbs_id=b.id AND trim(reported_name)<>'')>1`,
		"software_conflicts": `SELECT count(*) FROM bbs b WHERE (SELECT count(DISTINCT lower(trim(reported_software))) FROM source_entry s WHERE s.bbs_id=b.id AND trim(reported_software)<>'')>1`,
		"country_conflicts": `SELECT count(*) FROM bbs b WHERE (SELECT count(DISTINCT lower(trim(reported_country))) FROM source_entry s WHERE s.bbs_id=b.id AND trim(reported_country)<>'')>1`,
		"description_conflicts": `SELECT count(*) FROM bbs b WHERE (SELECT count(DISTINCT lower(trim(reported_description))) FROM source_entry s WHERE s.bbs_id=b.id AND trim(reported_description)<>'')>1`,
	}
	out := map[string]int64{}
	for key, query := range queries {
		var count int64
		if err := s.db.QueryRowContext(r.Context(), query).Scan(&count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out[key] = count
	}
	jsonOut(w, out)
}
