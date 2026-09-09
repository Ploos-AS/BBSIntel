package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/cursor"
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

func normalizeSince(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05"), nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.UTC); err == nil {
		return t.Format("2006-01-02 15:04:05"), nil
	}
	return "", fmt.Errorf("invalid since timestamp")
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
	conditions := []string{}
	args := []any{}
	if bbsID != "" {
		conditions = append(conditions, "bbs_id=?")
		args = append(args, bbsID)
	}

	since, err := normalizeSince(r.URL.Query().Get("since"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if since != "" {
		conditions = append(conditions, "occurred_at>?")
		args = append(args, since)
	}

	if token := strings.TrimSpace(r.URL.Query().Get("cursor")); token != "" {
		parts, err := cursor.Decode(token, 2)
		if err != nil {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		conditions = append(conditions, "(occurred_at<? OR (occurred_at=? AND id<?))")
		args = append(args, parts[0], parts[0], id)
	}

	query := `SELECT id,COALESCE(bbs_id,0),COALESCE(endpoint_id,0),occurred_at,kind,source,field,old_value,new_value,detail FROM change_event`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY occurred_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
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
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		w.Header().Set("X-Next-Cursor", cursor.Encode(last.OccurredAt, strconv.FormatInt(last.ID, 10)))
	}
	jsonOut(w, out)
}
