package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Window struct {
	Checks       int     `json:"checks"`
	OnlineChecks int     `json:"online_checks"`
	UptimePct    float64 `json:"uptime_pct"`
}

type Summary struct {
	FirstSeen     string `json:"first_seen"`
	LastSeen      string `json:"last_seen"`
	FirstProbe    string `json:"first_probe"`
	LastProbe     string `json:"last_probe"`
	LastOnline    string `json:"last_online"`
	StatusChanges int    `json:"status_changes"`
	Uptime24h     Window `json:"uptime_24h"`
	Uptime7d      Window `json:"uptime_7d"`
	Uptime30d     Window `json:"uptime_30d"`
}

type HistoryEntry struct {
	EndpointID        int64   `json:"endpoint_id"`
	Protocol          string  `json:"protocol"`
	Hostname          string  `json:"hostname"`
	Port              int     `json:"port"`
	CheckedAt         string  `json:"checked_at"`
	Status            string  `json:"status"`
	ConnectMS         int64   `json:"connect_ms"`
	BannerBytes       int     `json:"banner_bytes"`
	ObservedSoftware  string  `json:"observed_software"`
	SoftwareConfidence float64 `json:"software_confidence"`
	Error             string  `json:"error,omitempty"`
}

func ForBBS(ctx context.Context, db *sql.DB, bbsID string, now time.Time) (Summary, error) {
	var out Summary
	if db == nil {
		return out, fmt.Errorf("analytics: nil database")
	}

	if err := db.QueryRowContext(ctx, `SELECT created_at FROM bbs WHERE id=?`, bbsID).Scan(&out.FirstSeen); err != nil {
		return out, err
	}
	_ = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(last_seen),'') FROM source_entry WHERE bbs_id=?`, bbsID).Scan(&out.LastSeen)
	_ = db.QueryRowContext(ctx, `
SELECT COALESCE(MIN(p.checked_at),''),COALESCE(MAX(p.checked_at),''),
       COALESCE(MAX(CASE WHEN p.status='online' THEN p.checked_at END),'')
FROM probe_result p JOIN endpoint e ON e.id=p.endpoint_id WHERE e.bbs_id=?`, bbsID).Scan(&out.FirstProbe, &out.LastProbe, &out.LastOnline)

	changes, err := statusChanges(ctx, db, bbsID)
	if err != nil {
		return out, err
	}
	out.StatusChanges = changes

	if out.Uptime24h, err = window(ctx, db, bbsID, now.Add(-24*time.Hour)); err != nil {
		return out, err
	}
	if out.Uptime7d, err = window(ctx, db, bbsID, now.Add(-7*24*time.Hour)); err != nil {
		return out, err
	}
	if out.Uptime30d, err = window(ctx, db, bbsID, now.Add(-30*24*time.Hour)); err != nil {
		return out, err
	}
	return out, nil
}

func History(ctx context.Context, db *sql.DB, bbsID string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx, `
SELECT e.id,e.protocol,e.hostname,e.port,p.checked_at,p.status,
       COALESCE(p.connect_ms,0),p.banner_bytes,COALESCE(p.detected_software,''),
       COALESCE(p.software_confidence,0),COALESCE(p.error,'')
FROM probe_result p JOIN endpoint e ON e.id=p.endpoint_id
WHERE e.bbs_id=? ORDER BY p.checked_at DESC,p.id DESC LIMIT ?`, bbsID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]HistoryEntry, 0)
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.EndpointID, &h.Protocol, &h.Hostname, &h.Port, &h.CheckedAt, &h.Status,
			&h.ConnectMS, &h.BannerBytes, &h.ObservedSoftware, &h.SoftwareConfidence, &h.Error); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func window(ctx context.Context, db *sql.DB, bbsID string, since time.Time) (Window, error) {
	var w Window
	err := db.QueryRowContext(ctx, `
SELECT count(*),COALESCE(SUM(CASE WHEN p.status='online' THEN 1 ELSE 0 END),0)
FROM probe_result p JOIN endpoint e ON e.id=p.endpoint_id
WHERE e.bbs_id=? AND p.checked_at>=?`, bbsID, since.UTC().Format("2006-01-02 15:04:05")).Scan(&w.Checks, &w.OnlineChecks)
	if err != nil {
		return w, err
	}
	if w.Checks > 0 {
		w.UptimePct = float64(w.OnlineChecks) * 100 / float64(w.Checks)
	}
	return w, nil
}

func statusChanges(ctx context.Context, db *sql.DB, bbsID string) (int, error) {
	rows, err := db.QueryContext(ctx, `
SELECT e.id,p.status FROM probe_result p JOIN endpoint e ON e.id=p.endpoint_id
WHERE e.bbs_id=? ORDER BY e.id,p.checked_at,p.id`, bbsID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	last := map[int64]string{}
	changes := 0
	for rows.Next() {
		var endpointID int64
		var status string
		if err := rows.Scan(&endpointID, &status); err != nil {
			return 0, err
		}
		if prev, ok := last[endpointID]; ok && prev != status {
			changes++
		}
		last[endpointID] = status
	}
	return changes, rows.Err()
}
