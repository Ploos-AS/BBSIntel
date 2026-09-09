package publicstats

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Dimension struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type LifecycleDay struct {
	Date           string `json:"date"`
	NewBBS         int64  `json:"new_bbs"`
	DisappearedBBS int64  `json:"disappeared_bbs"`
	ReturnedBBS    int64  `json:"returned_bbs"`
}

// Refresh materializes inexpensive, cache-friendly public statistics from
// canonical inventory/source state. It also tracks whole-BBS presence
// transitions so a BBS is only considered disappeared when no active source
// entry remains.
func Refresh(ctx context.Context, db *sql.DB, now time.Time) error {
	if db == nil {
		return fmt.Errorf("publicstats: nil database")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stamp := now.UTC().Format("2006-01-02 15:04:05")
	if _, err := tx.ExecContext(ctx, `DELETE FROM public_dimension_snapshot`); err != nil {
		return err
	}
	for _, q := range []string{
		`INSERT INTO public_dimension_snapshot(dimension,value,count,refreshed_at)
SELECT 'software',trim(software),count(*),? FROM bbs WHERE trim(software)<>'' GROUP BY trim(software)`,
		`INSERT INTO public_dimension_snapshot(dimension,value,count,refreshed_at)
SELECT 'country',trim(country),count(*),? FROM bbs WHERE trim(country)<>'' GROUP BY trim(country)`,
		`INSERT INTO public_dimension_snapshot(dimension,value,count,refreshed_at)
SELECT 'protocol',lower(trim(protocol)),count(*),? FROM endpoint WHERE trim(protocol)<>'' GROUP BY lower(trim(protocol))`,
		`INSERT INTO public_dimension_snapshot(dimension,value,count,refreshed_at)
SELECT 'source',source,count(DISTINCT bbs_id),? FROM source_entry WHERE active=1 GROUP BY source`,
	} {
		if _, err := tx.ExecContext(ctx, q, stamp); err != nil {
			return fmt.Errorf("publicstats dimension: %w", err)
		}
	}

	// Backfill new-BBS history from immutable creation timestamps.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO public_lifecycle_daily(day,new_bbs,disappeared_bbs,returned_bbs)
SELECT substr(created_at,1,10),count(*),0,0 FROM bbs GROUP BY substr(created_at,1,10)
ON CONFLICT(day) DO UPDATE SET new_bbs=excluded.new_bbs`); err != nil {
		return fmt.Errorf("publicstats new bbs: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
SELECT b.id,CASE WHEN EXISTS(SELECT 1 FROM source_entry s WHERE s.bbs_id=b.id AND s.active=1) THEN 1 ELSE 0 END,
       COALESCE(p.active,-1)
FROM bbs b LEFT JOIN public_bbs_presence p ON p.bbs_id=b.id`)
	if err != nil {
		return err
	}
	type presence struct{ id int64; active, previous int }
	var states []presence
	for rows.Next() {
		var p presence
		if err := rows.Scan(&p.id, &p.active, &p.previous); err != nil {
			rows.Close()
			return err
		}
		states = append(states, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	day := now.UTC().Format("2006-01-02")
	for _, p := range states {
		if p.previous == -1 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO public_bbs_presence(bbs_id,active,last_changed) VALUES(?,?,?)`, p.id, p.active, stamp); err != nil {
				return err
			}
			continue
		}
		if p.active == p.previous {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE public_bbs_presence SET active=?,last_changed=? WHERE bbs_id=?`, p.active, stamp, p.id); err != nil {
			return err
		}
		column := "returned_bbs"
		if p.active == 0 {
			column = "disappeared_bbs"
		}
		q := `INSERT INTO public_lifecycle_daily(day,new_bbs,disappeared_bbs,returned_bbs) VALUES(?,0,0,0)
ON CONFLICT(day) DO NOTHING`
		if _, err := tx.ExecContext(ctx, q, day); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE public_lifecycle_daily SET `+column+`=`+column+`+1 WHERE day=?`, day); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func Dimensions(ctx context.Context, db *sql.DB, dimension string) ([]Dimension, error) {
	rows, err := db.QueryContext(ctx, `SELECT value,count FROM public_dimension_snapshot WHERE dimension=? ORDER BY count DESC,lower(value),value`, dimension)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dimension{}
	for rows.Next() {
		var d Dimension
		if err := rows.Scan(&d.Value, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func Lifecycle(ctx context.Context, db *sql.DB, days int, now time.Time) ([]LifecycleDay, error) {
	if days <= 0 {
		days = 30
	}
	if days > 3650 {
		days = 3650
	}
	since := now.UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `SELECT day,new_bbs,disappeared_bbs,returned_bbs FROM public_lifecycle_daily WHERE day>=? ORDER BY day`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LifecycleDay{}
	for rows.Next() {
		var d LifecycleDay
		if err := rows.Scan(&d.Date, &d.NewBBS, &d.DisappearedBBS, &d.ReturnedBBS); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
