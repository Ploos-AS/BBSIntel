package rollup

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Refresh incrementally materializes raw probe history into hourly and daily
// buckets. The first run backfills existing history; later runs only rebuild
// recent buckets so the cost stays bounded as probe_result grows.
func Refresh(ctx context.Context, db *sql.DB, now time.Time) error {
	if db == nil {
		return fmt.Errorf("rollup: nil database")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var hourlyRows int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM endpoint_hourly`).Scan(&hourlyRows); err != nil {
		return err
	}
	hourlySince := ""
	if hourlyRows > 0 {
		hourlySince = now.UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
		if _, err := tx.ExecContext(ctx, `DELETE FROM endpoint_hourly WHERE bucket_start>=?`, hourlySince); err != nil {
			return err
		}
	}

	hourlyWhere := ""
	args := []any{}
	if hourlySince != "" {
		hourlyWhere = "WHERE checked_at>=?"
		args = append(args, hourlySince)
	}
	q := `INSERT INTO endpoint_hourly (
 endpoint_id,bucket_start,checks,online_checks,telnet_only_checks,tcp_only_checks,offline_checks,dns_fail_checks,connect_ms_sum,connect_ms_count
)
SELECT endpoint_id,substr(checked_at,1,13)||':00:00',count(*),
 SUM(CASE WHEN status='online' THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='telnet_only' THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='tcp_only' THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='offline' THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='dns_fail' THEN 1 ELSE 0 END),
 COALESCE(SUM(CASE WHEN connect_ms IS NOT NULL THEN connect_ms ELSE 0 END),0),
 SUM(CASE WHEN connect_ms IS NOT NULL THEN 1 ELSE 0 END)
FROM probe_result ` + hourlyWhere + `
GROUP BY endpoint_id,substr(checked_at,1,13)`
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("rollup hourly: %w", err)
	}

	var dailyRows int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM endpoint_daily`).Scan(&dailyRows); err != nil {
		return err
	}
	dailySince := ""
	if dailyRows > 0 {
		dailySince = now.UTC().AddDate(0, 0, -35).Format("2006-01-02 00:00:00")
		if _, err := tx.ExecContext(ctx, `DELETE FROM endpoint_daily WHERE bucket_start>=?`, dailySince); err != nil {
			return err
		}
	}

	dailyWhere := ""
	args = nil
	if dailySince != "" {
		dailyWhere = "WHERE bucket_start>=?"
		args = append(args, dailySince)
	}
	q = `INSERT INTO endpoint_daily (
 endpoint_id,bucket_start,checks,online_checks,telnet_only_checks,tcp_only_checks,offline_checks,dns_fail_checks,connect_ms_sum,connect_ms_count
)
SELECT endpoint_id,substr(bucket_start,1,10)||' 00:00:00',
 SUM(checks),SUM(online_checks),SUM(telnet_only_checks),SUM(tcp_only_checks),SUM(offline_checks),SUM(dns_fail_checks),SUM(connect_ms_sum),SUM(connect_ms_count)
FROM endpoint_hourly ` + dailyWhere + `
GROUP BY endpoint_id,substr(bucket_start,1,10)`
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("rollup daily: %w", err)
	}

	return tx.Commit()
}

// PruneRaw removes raw probe rows older than retention. Callers should invoke
// Refresh first so long-term daily statistics survive raw-history pruning.
func PruneRaw(ctx context.Context, db *sql.DB, now time.Time, retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := now.UTC().Add(-retention).Format("2006-01-02 15:04:05")
	res, err := db.ExecContext(ctx, `DELETE FROM probe_result WHERE checked_at<?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
