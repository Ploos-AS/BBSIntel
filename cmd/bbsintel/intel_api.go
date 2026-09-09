package main

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/intel"
)

func registerIntelRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/bbs/{id}/intelligence", srv.getBBSIntelligence)
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
