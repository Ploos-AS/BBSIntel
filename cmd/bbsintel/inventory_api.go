package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/cursor"
	"github.com/Ploos-AS/BBSIntel/internal/publicstats"
)

const (
	defaultBBSPageSize = 50
	maxBBSPageSize     = 100
)

type bbsListFilters struct {
	Query    string
	Software string
	Country  string
	Protocol string
	Source   string
}

func (s *server) listBBS(w http.ResponseWriter, r *http.Request) {
	limit, filters, cursorToken, err := parseBBSListQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	lastName := ""
	lastID := int64(0)
	if cursorToken != "" {
		parts, err := cursor.Decode(cursorToken, 8)
		if err != nil || parts[0] != "v1" {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		if parts[3] != filters.Query || parts[4] != filters.Software || parts[5] != filters.Country || parts[6] != filters.Protocol || parts[7] != filters.Source {
			http.Error(w, "cursor does not match query filters", http.StatusBadRequest)
			return
		}
		lastName = parts[1]
		lastID, err = strconv.ParseInt(parts[2], 10, 64)
		if err != nil || lastID < 0 {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
	}

	rows, err := s.db.QueryContext(r.Context(), `
SELECT b.id,b.name,b.software,b.country,
       COALESCE(e.protocol,''),COALESCE(e.hostname,''),COALESCE(e.port,0),
       COALESCE(pr.status,''),COALESCE(pr.checked_at,''),COALESCE(pr.connect_ms,0),
       COALESCE(pr.detected_software,''),COALESCE(pr.software_confidence,0),COALESCE(pr.software_evidence,'')
FROM bbs b
LEFT JOIN endpoint e ON e.id=(SELECT e2.id FROM endpoint e2 WHERE e2.bbs_id=b.id ORDER BY e2.id LIMIT 1)
LEFT JOIN probe_result pr ON pr.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
WHERE (?='' OR instr(lower(b.name),?)>0)
  AND (?='' OR lower(trim(b.software))=?)
  AND (?='' OR lower(trim(b.country))=?)
  AND (?='' OR EXISTS(SELECT 1 FROM endpoint ep WHERE ep.bbs_id=b.id AND lower(trim(ep.protocol))=?))
  AND (?='' OR EXISTS(SELECT 1 FROM source_entry se WHERE se.bbs_id=b.id AND se.active=1 AND lower(trim(se.source))=?))
  AND (?='' OR lower(b.name)>? OR (lower(b.name)=? AND b.id>?))
ORDER BY lower(b.name),b.id
LIMIT ?`,
		filters.Query, filters.Query,
		filters.Software, filters.Software,
		filters.Country, filters.Country,
		filters.Protocol, filters.Protocol,
		filters.Source, filters.Source,
		lastName, lastName, lastName, lastID,
		limit+1,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	var nextName string
	var nextID int64
	for rows.Next() {
		var id int64
		var name, reportedSoftware, country, protocol, hostname, status, checkedAt, observedSoftware, evidence string
		var port int
		var connectMS int64
		var confidence float64
		if err := rows.Scan(&id, &name, &reportedSoftware, &country, &protocol, &hostname, &port, &status, &checkedAt, &connectMS, &observedSoftware, &confidence, &evidence); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(out) == limit {
			nextName = strings.ToLower(name)
			nextID = id
			break
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "reported_software": reportedSoftware, "observed_software": observedSoftware,
			"software_confidence": confidence, "software_evidence": evidence,
			"software_mismatch": softwareMismatch(reportedSoftware, observedSoftware), "country": country,
			"protocol": protocol, "hostname": hostname, "port": port,
			"status": status, "checked_at": checkedAt, "connect_ms": connectMS,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nextName != "" {
		last := out[len(out)-1]
		w.Header().Set("X-Next-Cursor", cursor.Encode("v1", strings.ToLower(last["name"].(string)), strconv.FormatInt(last["id"].(int64), 10), filters.Query, filters.Software, filters.Country, filters.Protocol, filters.Source))
		_ = nextID
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	jsonOut(w, out)
}

func parseBBSListQuery(r *http.Request) (int, bbsListFilters, string, error) {
	allowed := map[string]bool{"limit": true, "cursor": true, "q": true, "software": true, "country": true, "protocol": true, "source": true}
	for key, values := range r.URL.Query() {
		if !allowed[key] {
			return 0, bbsListFilters{}, "", fmt.Errorf("unknown query parameter %q", key)
		}
		if len(values) != 1 {
			return 0, bbsListFilters{}, "", fmt.Errorf("query parameter %q must occur once", key)
		}
	}

	limit := defaultBBSPageSize
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxBBSPageSize {
			return 0, bbsListFilters{}, "", fmt.Errorf("limit must be between 1 and %d", maxBBSPageSize)
		}
		limit = n
	}

	f := bbsListFilters{
		Query:    strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))),
		Software: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("software"))),
		Country:  strings.ToLower(strings.TrimSpace(r.URL.Query().Get("country"))),
		Protocol: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("protocol"))),
		Source:   strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source"))),
	}
	for name, value := range map[string]string{"q": f.Query, "software": f.Software, "country": f.Country, "protocol": f.Protocol, "source": f.Source} {
		max := 64
		if name == "q" {
			max = 100
		}
		if len(value) > max {
			return 0, bbsListFilters{}, "", fmt.Errorf("%s is too long", name)
		}
	}
	return limit, f, strings.TrimSpace(r.URL.Query().Get("cursor")), nil
}

func (s *server) getBBS(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var name, reportedSoftware, country, description string
	err := s.db.QueryRowContext(r.Context(), `SELECT name,software,country,description FROM bbs WHERE id=?`, id).Scan(&name, &reportedSoftware, &country, &description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
SELECT e.id,e.protocol,e.hostname,e.port,
       COALESCE(pr.status,''),COALESCE(pr.checked_at,''),COALESCE(pr.connect_ms,0),COALESCE(pr.banner_bytes,0),
       COALESCE(pr.banner_sha256,''),COALESCE(pr.banner_preview,''),COALESCE(pr.detected_software,''),
       COALESCE(pr.software_confidence,0),COALESCE(pr.software_evidence,''),COALESCE(pr.error,'')
FROM endpoint e
LEFT JOIN probe_result pr ON pr.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
WHERE e.bbs_id=? ORDER BY e.id`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	endpoints := []map[string]any{}
	observedSoftware := ""
	maxConfidence := 0.0
	for rows.Next() {
		var endpointID int64
		var protocol, hostname, status, checkedAt, bannerSHA, bannerPreview, detectedSoftware, evidence, probeError string
		var port, bannerBytes int
		var connectMS int64
		var confidence float64
		if err := rows.Scan(&endpointID, &protocol, &hostname, &port, &status, &checkedAt, &connectMS, &bannerBytes, &bannerSHA, &bannerPreview, &detectedSoftware, &confidence, &evidence, &probeError); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if confidence > maxConfidence && detectedSoftware != "" {
			observedSoftware = detectedSoftware
			maxConfidence = confidence
		}
		endpoints = append(endpoints, map[string]any{
			"id": endpointID, "protocol": protocol, "hostname": hostname, "port": port,
			"status": status, "checked_at": checkedAt, "connect_ms": connectMS,
			"banner_bytes": bannerBytes, "banner_sha256": bannerSHA, "banner_preview": bannerPreview,
			"observed_software": detectedSoftware, "software_confidence": confidence,
			"software_evidence": evidence, "error": probeError,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{
		"id": id, "name": name, "reported_software": reportedSoftware, "observed_software": observedSoftware,
		"software_confidence": maxConfidence, "software_mismatch": softwareMismatch(reportedSoftware, observedSoftware),
		"country": country, "description": description, "endpoints": endpoints,
	})
}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	snapshot, err := publicstats.Runtime(r.Context(), s.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	jsonOut(w, snapshot)
}
