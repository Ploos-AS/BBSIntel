package sourcehealth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func EvaluateAll(ctx context.Context, db *sql.DB, now time.Time) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("source health: nil database")
	}
	rows, err := db.QueryContext(ctx, `
SELECT source,last_success_at,consecutive_failures,current_state
FROM source_health ORDER BY source`)
	if err != nil {
		return 0, err
	}
	type row struct {
		source       string
		lastSuccess  string
		failures     int
		currentState string
	}
	var items []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.source, &r.lastSuccess, &r.failures, &r.currentState); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	changed := 0
	for _, item := range items {
		var lastSuccess time.Time
		if item.lastSuccess != "" {
			lastSuccess, err = time.Parse("2006-01-02 15:04:05", item.lastSuccess)
			if err != nil {
				return changed, fmt.Errorf("source %s: parse last success: %w", item.source, err)
			}
		}
		next := Classify(now.UTC(), lastSuccess, item.failures).State
		if next == item.currentState {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return changed, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE source_health SET current_state=?,state_changed_at=CURRENT_TIMESTAMP WHERE source=?`, next, item.source); err != nil {
			tx.Rollback()
			return changed, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO change_event(kind,source,field,old_value,new_value,detail)
VALUES('source_health_changed',?,'freshness',?,?,?)`, item.source, item.currentState, next, "source health classification changed"); err != nil {
			tx.Rollback()
			return changed, err
		}
		if err := tx.Commit(); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
