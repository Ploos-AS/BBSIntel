package publicstats

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type RuntimeSnapshot struct {
	BBS                int64            `json:"bbs"`
	Endpoints          int64            `json:"endpoints"`
	Probes             int64            `json:"probes"`
	SoftwareMismatches int64            `json:"software_mismatches"`
	Status             map[string]int64 `json:"status"`
	RefreshedAt        string           `json:"refreshed_at"`
}

// RefreshRuntime moves expensive latest-probe aggregation out of the public
// request path. The worker pays this cost once per probe pass; /api/v1/stats
// then reads a tiny materialized snapshot.
func RefreshRuntime(ctx context.Context, db *sql.DB, now time.Time) error {
	if db == nil {
		return fmt.Errorf("publicstats runtime: nil database")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var bbs, endpoints, probes, mismatches int64
	for _, q := range []struct {
		sql string
		dst *int64
	}{
		{`SELECT count(*) FROM bbs`, &bbs},
		{`SELECT count(*) FROM endpoint`, &endpoints},
		{`SELECT count(*) FROM probe_result`, &probes},
		{`SELECT count(*) FROM bbs b
WHERE trim(b.software)<>'' AND EXISTS (
 SELECT 1 FROM endpoint e JOIN probe_result p ON p.endpoint_id=e.id
 WHERE e.bbs_id=b.id AND trim(p.detected_software)<>''
   AND lower(trim(p.detected_software))<>lower(trim(b.software))
   AND p.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
)`, &mismatches},
	} {
		if err := tx.QueryRowContext(ctx, q.sql).Scan(q.dst); err != nil {
			return fmt.Errorf("publicstats runtime count: %w", err)
		}
	}

	status := map[string]int64{}
	rows, err := tx.QueryContext(ctx, `
SELECT p.status,count(*)
FROM endpoint e
JOIN probe_result p ON p.id=(SELECT p2.id FROM probe_result p2 WHERE p2.endpoint_id=e.id ORDER BY p2.checked_at DESC,p2.id DESC LIMIT 1)
GROUP BY p.status`)
	if err != nil {
		return fmt.Errorf("publicstats runtime status: %w", err)
	}
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			rows.Close()
			return err
		}
		status[name] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	stamp := now.UTC().Format("2006-01-02 15:04:05")
	if _, err := tx.ExecContext(ctx, `DELETE FROM public_runtime_snapshot`); err != nil {
		return err
	}
	metrics := map[string]int64{
		"bbs":                 bbs,
		"endpoints":           endpoints,
		"probes":              probes,
		"software_mismatches": mismatches,
	}
	for key, value := range metrics {
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_runtime_snapshot(metric,value,refreshed_at) VALUES(?,?,?)`, key, value, stamp); err != nil {
			return err
		}
	}
	for name, value := range status {
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_runtime_snapshot(metric,value,refreshed_at) VALUES(?,?,?)`, "status:"+name, value, stamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func Runtime(ctx context.Context, db *sql.DB) (RuntimeSnapshot, error) {
	out := RuntimeSnapshot{Status: map[string]int64{}}
	rows, err := db.QueryContext(ctx, `SELECT metric,value,refreshed_at FROM public_runtime_snapshot ORDER BY metric`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var metric, refreshed string
		var value int64
		if err := rows.Scan(&metric, &value, &refreshed); err != nil {
			return out, err
		}
		if refreshed > out.RefreshedAt {
			out.RefreshedAt = refreshed
		}
		switch metric {
		case "bbs":
			out.BBS = value
		case "endpoints":
			out.Endpoints = value
		case "probes":
			out.Probes = value
		case "software_mismatches":
			out.SoftwareMismatches = value
		default:
			const prefix = "status:"
			if len(metric) > len(prefix) && metric[:len(prefix)] == prefix {
				out.Status[metric[len(prefix):]] = value
			}
		}
	}
	return out, rows.Err()
}
