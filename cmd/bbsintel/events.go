package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

type eventView struct {
	ID         int64  `json:"id"`
	BBSID      int64  `json:"bbs_id"`
	EndpointID int64  `json:"endpoint_id,omitempty"`
	OccurredAt string `json:"occurred_at"`
	Kind       string `json:"kind"`
	Source     string `json:"source,omitempty"`
	Field      string `json:"field,omitempty"`
	OldValue   string `json:"old_value,omitempty"`
	NewValue   string `json:"new_value,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func registerEventRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/v1/events", srv.listEvents)
	mux.HandleFunc("GET /api/v1/bbs/{id}/events", srv.listBBSEvents)
}

func eventLimit(r *http.Request) int {
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	return limit
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	s.writeEvents(w, r, "", eventLimit(r))
}

func (s *server) listBBSEvents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM bbs WHERE id=?`, id).Scan(&exists); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeEvents(w, r, id, eventLimit(r))
}

func (s *server) writeEvents(w http.ResponseWriter, r *http.Request, bbsID string, limit int) {
	query := `SELECT id,COALESCE(bbs_id,0),COALESCE(endpoint_id,0),occurred_at,kind,source,field,old_value,new_value,detail FROM change_event`
	args := []any{}
	if bbsID != "" {
		query += ` WHERE bbs_id=?`
		args = append(args, bbsID)
	}
	query += ` ORDER BY occurred_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []eventView{}
	for rows.Next() {
		var v eventView
		if err := rows.Scan(&v.ID, &v.BBSID, &v.EndpointID, &v.OccurredAt, &v.Kind, &v.Source, &v.Field, &v.OldValue, &v.NewValue, &v.Detail); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, v)
	}
	jsonOut(w, out)
}
