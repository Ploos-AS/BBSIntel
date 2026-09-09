package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type Alert struct {
	Key        string `json:"key"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Source     string `json:"source,omitempty"`
	BBSID      int64  `json:"bbs_id,omitempty"`
	EndpointID int64  `json:"endpoint_id,omitempty"`
	OccurredAt string `json:"occurred_at,omitempty"`
	Title      string `json:"title"`
	Detail     string `json:"detail,omitempty"`
}

type Filter struct {
	Limit           int
	MinimumSeverity string
	Category        string
	Source          string
	BBSID           int64
}

type Stats struct {
	Total      int            `json:"total"`
	BySeverity map[string]int `json:"by_severity"`
	ByCategory map[string]int `json:"by_category"`
	BySource   map[string]int `json:"by_source"`
}

var severityRank = map[string]int{"info": 0, "warning": 1, "high": 2, "critical": 3}

func List(ctx context.Context, db *sql.DB, limit int, minimumSeverity string) ([]Alert, error) {
	return Query(ctx, db, Filter{Limit: limit, MinimumSeverity: minimumSeverity})
}

func Query(ctx context.Context, db *sql.DB, filter Filter) ([]Alert, error) {
	all, err := collect(ctx, db)
	if err != nil {
		return nil, err
	}
	filtered := applyFilter(all, filter)
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func Summarize(ctx context.Context, db *sql.DB, filter Filter) (Stats, error) {
	all, err := collect(ctx, db)
	if err != nil {
		return Stats{}, err
	}
	filter.Limit = 0
	filtered := applyFilter(all, filter)
	out := Stats{
		Total:      len(filtered),
		BySeverity: map[string]int{"info": 0, "warning": 0, "high": 0, "critical": 0},
		ByCategory: map[string]int{},
		BySource:   map[string]int{},
	}
	for _, alert := range filtered {
		out.BySeverity[alert.Severity]++
		out.ByCategory[alert.Category]++
		if alert.Source != "" {
			out.BySource[alert.Source]++
		}
	}
	return out, nil
}

func collect(ctx context.Context, db *sql.DB) ([]Alert, error) {
	if db == nil {
		return nil, fmt.Errorf("alerts: nil database")
	}
	var out []Alert
	if err := appendSourceHealth(ctx, db, &out); err != nil {
		return nil, err
	}
	if err := appendMissingSources(ctx, db, &out); err != nil {
		return nil, err
	}
	if err := appendEndpointFailures(ctx, db, &out); err != nil {
		return nil, err
	}
	if err := appendSoftwareChanges(ctx, db, &out); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank[out[i].Severity], severityRank[out[j].Severity]
		if ri != rj {
			return ri > rj
		}
		if out[i].OccurredAt != out[j].OccurredAt {
			return out[i].OccurredAt > out[j].OccurredAt
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func applyFilter(in []Alert, filter Filter) []Alert {
	minRank, ok := severityRank[strings.ToLower(strings.TrimSpace(filter.MinimumSeverity))]
	if !ok {
		minRank = 0
	}
	category := strings.ToLower(strings.TrimSpace(filter.Category))
	source := strings.ToLower(strings.TrimSpace(filter.Source))
	out := make([]Alert, 0, len(in))
	for _, alert := range in {
		if severityRank[alert.Severity] < minRank {
			continue
		}
		if category != "" && strings.ToLower(alert.Category) != category {
			continue
		}
		if source != "" && strings.ToLower(alert.Source) != source {
			continue
		}
		if filter.BBSID > 0 && alert.BBSID != filter.BBSID {
			continue
		}
		out = append(out, alert)
	}
	return out
}

func appendSourceHealth(ctx context.Context, db *sql.DB, out *[]Alert) error {
	rows, err := db.QueryContext(ctx, `SELECT source,current_state,state_changed_at,last_error FROM source_health WHERE current_state IN ('degraded','stale','failed') ORDER BY source`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, state, changedAt, lastError string
		if err := rows.Scan(&source, &state, &changedAt, &lastError); err != nil {
			return err
		}
		severity := "warning"
		if state == "stale" {
			severity = "high"
		} else if state == "failed" {
			severity = "critical"
		}
		*out = append(*out, Alert{Key: "source-health:" + source, Severity: severity, Category: "source_health", Source: source, OccurredAt: changedAt, Title: "Source is " + state, Detail: lastError})
	}
	return rows.Err()
}

func appendMissingSources(ctx context.Context, db *sql.DB, out *[]Alert) error {
	rows, err := db.QueryContext(ctx, `SELECT bbs_id,source,source_key,missing_since FROM source_entry WHERE active=0 ORDER BY missing_since DESC,id DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var bbsID int64
		var source, sourceKey, missingSince string
		if err := rows.Scan(&bbsID, &source, &sourceKey, &missingSince); err != nil {
			return err
		}
		*out = append(*out, Alert{Key: "source-missing:" + source + ":" + sourceKey, Severity: "warning", Category: "source_missing", Source: source, BBSID: bbsID, OccurredAt: missingSince, Title: "BBS disappeared from source", Detail: sourceKey})
	}
	return rows.Err()
}

func appendEndpointFailures(ctx context.Context, db *sql.DB, out *[]Alert) error {
	rows, err := db.QueryContext(ctx, `
SELECT e.id,e.bbs_id,e.protocol,e.hostname,e.port,p.status,p.checked_at,COALESCE(p.error,'')
FROM endpoint e JOIN probe_result p ON p.id=(
 SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1
)
WHERE p.status IN ('offline','dns_fail') ORDER BY p.checked_at DESC,e.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var endpointID, bbsID int64
		var protocol, hostname, status, checkedAt, probeError string
		var port int
		if err := rows.Scan(&endpointID, &bbsID, &protocol, &hostname, &port, &status, &checkedAt, &probeError); err != nil {
			return err
		}
		severity := "high"
		if status == "dns_fail" {
			severity = "warning"
		}
		*out = append(*out, Alert{Key: fmt.Sprintf("endpoint:%d", endpointID), Severity: severity, Category: "endpoint_status", BBSID: bbsID, EndpointID: endpointID, OccurredAt: checkedAt, Title: "Endpoint is " + status, Detail: fmt.Sprintf("%s://%s:%d %s", protocol, hostname, port, probeError)})
	}
	return rows.Err()
}

func appendSoftwareChanges(ctx context.Context, db *sql.DB, out *[]Alert) error {
	rows, err := db.QueryContext(ctx, `SELECT id,COALESCE(bbs_id,0),COALESCE(endpoint_id,0),occurred_at,old_value,new_value,detail FROM change_event WHERE kind='software_changed' ORDER BY occurred_at DESC,id DESC LIMIT 1000`)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := map[int64]struct{}{}
	for rows.Next() {
		var eventID, bbsID, endpointID int64
		var occurredAt, oldValue, newValue, detail string
		if err := rows.Scan(&eventID, &bbsID, &endpointID, &occurredAt, &oldValue, &newValue, &detail); err != nil {
			return err
		}
		if _, ok := seen[endpointID]; ok {
			continue
		}
		seen[endpointID] = struct{}{}
		*out = append(*out, Alert{Key: fmt.Sprintf("software-change:%d", endpointID), Severity: "info", Category: "software_change", BBSID: bbsID, EndpointID: endpointID, OccurredAt: occurredAt, Title: "Observed software changed", Detail: oldValue + " -> " + newValue + " " + detail})
		_ = eventID
	}
	return rows.Err()
}
