package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/source"
)

func Import(ctx context.Context, db *sql.DB, adapter source.Adapter) (int, error) {
	started := time.Now()
	sourceName := strings.TrimSpace(adapter.Name())
	if sourceName == "" {
		return 0, fmt.Errorf("source adapter has empty name")
	}

	entries, err := adapter.Fetch(ctx)
	if err != nil {
		recordSourceFailure(ctx, db, sourceName, started, err)
		return 0, err
	}
	if len(entries) == 0 {
		err := fmt.Errorf("source %s returned an empty snapshot; refusing presence reconciliation", sourceName)
		recordSourceFailure(ctx, db, sourceName, started, err)
		return 0, err
	}

	seen := make(map[string]struct{})
	for i := range entries {
		if strings.TrimSpace(entries[i].Source) == "" {
			entries[i].Source = sourceName
		}
		if entries[i].Source != sourceName {
			err := fmt.Errorf("source adapter %s returned entry for source %s", sourceName, entries[i].Source)
			recordSourceFailure(ctx, db, sourceName, started, err)
			return 0, err
		}
		if strings.TrimSpace(entries[i].SourceKey) == "" {
			err := fmt.Errorf("source %s returned entry with empty source key", sourceName)
			recordSourceFailure(ctx, db, sourceName, started, err)
			return 0, err
		}
		seen[entries[i].SourceKey] = struct{}{}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		recordSourceFailure(ctx, db, sourceName, started, err)
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if err := upsert(ctx, tx, e); err != nil {
			_ = tx.Rollback()
			recordSourceFailure(ctx, db, sourceName, started, err)
			return count, err
		}
		count++
	}
	if err := reconcilePresence(ctx, tx, sourceName, seen); err != nil {
		_ = tx.Rollback()
		recordSourceFailure(ctx, db, sourceName, started, err)
		return count, err
	}
	if err := recordSourceSuccess(ctx, tx, sourceName, started, count); err != nil {
		_ = tx.Rollback()
		recordSourceFailure(ctx, db, sourceName, started, err)
		return count, err
	}
	if err := tx.Commit(); err != nil {
		recordSourceFailure(ctx, db, sourceName, started, err)
		return count, err
	}
	return count, nil
}

func recordSourceSuccess(ctx context.Context, tx *sql.Tx, sourceName string, started time.Time, count int) error {
	durationMS := time.Since(started).Milliseconds()
	_, err := tx.ExecContext(ctx, `INSERT INTO source_health(
 source,last_attempt_at,last_success_at,last_duration_ms,last_entry_count,consecutive_failures,last_error
) VALUES(?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,?,?,0,'')
ON CONFLICT(source) DO UPDATE SET
 last_attempt_at=CURRENT_TIMESTAMP,last_success_at=CURRENT_TIMESTAMP,last_duration_ms=excluded.last_duration_ms,
 last_entry_count=excluded.last_entry_count,consecutive_failures=0,last_error=''`, sourceName, durationMS, count)
	return err
}

func recordSourceFailure(ctx context.Context, db *sql.DB, sourceName string, started time.Time, importErr error) {
	if db == nil || sourceName == "" || importErr == nil {
		return
	}
	durationMS := time.Since(started).Milliseconds()
	_, _ = db.ExecContext(ctx, `INSERT INTO source_health(
 source,last_attempt_at,last_failure_at,last_duration_ms,last_entry_count,consecutive_failures,last_error
) VALUES(?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,?,0,1,?)
ON CONFLICT(source) DO UPDATE SET
 last_attempt_at=CURRENT_TIMESTAMP,last_failure_at=CURRENT_TIMESTAMP,last_duration_ms=excluded.last_duration_ms,
 consecutive_failures=source_health.consecutive_failures+1,last_error=excluded.last_error`, sourceName, durationMS, importErr.Error())
}

func upsert(ctx context.Context, tx *sql.Tx, e source.Entry) error {
	e.Hostname = strings.ToLower(strings.TrimSpace(e.Hostname))
	var bbsID int64
	var sourceActive int
	err := tx.QueryRowContext(ctx, `SELECT bbs_id,active FROM source_entry WHERE source=? AND source_key=?`, e.Source, e.SourceKey).Scan(&bbsID, &sourceActive)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	newSource := err == sql.ErrNoRows
	newBBS := false
	if newSource {
		err = tx.QueryRowContext(ctx, `SELECT bbs_id FROM endpoint WHERE protocol=? AND lower(hostname)=lower(?) AND port=?`, e.Protocol, e.Hostname, e.Port).Scan(&bbsID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == sql.ErrNoRows {
			res, err := tx.ExecContext(ctx, `INSERT INTO bbs(name,software,country,description) VALUES(?,?,?,?)`, e.Name, e.Software, e.Country, e.Description)
			if err != nil {
				return err
			}
			bbsID, err = res.LastInsertId()
			if err != nil {
				return err
			}
			newBBS = true
		} else if err := fillCanonicalMetadata(ctx, tx, bbsID, e); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO source_entry(
 bbs_id,source,source_key,source_url,reported_name,reported_software,reported_country,reported_description,active,missing_since
) VALUES(?,?,?,?,?,?,?,?,1,'')`, bbsID, e.Source, e.SourceKey, e.SourceURL, e.Name, e.Software, e.Country, e.Description)
		if err != nil {
			return err
		}
		if newBBS {
			if err := addEvent(ctx, tx, bbsID, 0, "bbs_new", e.Source, "", "", e.Name, "discovered by directory import"); err != nil {
				return err
			}
		}
		if err := addEvent(ctx, tx, bbsID, 0, "source_added", e.Source, "", "", e.SourceKey, e.SourceURL); err != nil {
			return err
		}
	} else {
		if err := fillCanonicalMetadata(ctx, tx, bbsID, e); err != nil {
			return err
		}
		var oldURL, oldName, oldSoftware, oldCountry, oldDescription string
		if err := tx.QueryRowContext(ctx, `SELECT source_url,reported_name,reported_software,reported_country,reported_description
FROM source_entry WHERE source=? AND source_key=?`, e.Source, e.SourceKey).Scan(&oldURL, &oldName, &oldSoftware, &oldCountry, &oldDescription); err != nil {
			return err
		}
		for _, change := range []struct {
			field, old, new string
		}{
			{"source_url", oldURL, e.SourceURL},
			{"name", oldName, e.Name},
			{"software", oldSoftware, e.Software},
			{"country", oldCountry, e.Country},
			{"description", oldDescription, e.Description},
		} {
			if strings.TrimSpace(change.old) != strings.TrimSpace(change.new) {
				if err := addEvent(ctx, tx, bbsID, 0, "source_changed", e.Source, change.field, change.old, change.new, e.SourceKey); err != nil {
					return err
				}
			}
		}
		if sourceActive == 0 {
			if err := addEvent(ctx, tx, bbsID, 0, "source_returned", e.Source, "presence", "missing", "present", e.SourceKey); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE source_entry SET
 source_url=?,reported_name=?,reported_software=?,reported_country=?,reported_description=?,last_seen=CURRENT_TIMESTAMP,
 active=1,missing_since=''
 WHERE source=? AND source_key=?`, e.SourceURL, e.Name, e.Software, e.Country, e.Description, e.Source, e.SourceKey)
		if err != nil {
			return err
		}
	}

	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM endpoint WHERE protocol=? AND lower(hostname)=lower(?) AND port=?`, e.Protocol, e.Hostname, e.Port).Scan(&existingID)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, e.Protocol, e.Hostname, e.Port)
	if err != nil {
		return fmt.Errorf("endpoint %s:%d: %w", e.Hostname, e.Port, err)
	}
	endpointID, _ := res.LastInsertId()
	return addEvent(ctx, tx, bbsID, endpointID, "endpoint_added", e.Source, "endpoint", "", fmt.Sprintf("%s://%s:%d", e.Protocol, e.Hostname, e.Port), e.SourceKey)
}

func reconcilePresence(ctx context.Context, tx *sql.Tx, sourceName string, seen map[string]struct{}) error {
	rows, err := tx.QueryContext(ctx, `SELECT bbs_id,source_key FROM source_entry WHERE source=? AND active=1`, sourceName)
	if err != nil {
		return err
	}
	type missingEntry struct {
		bbsID     int64
		sourceKey string
	}
	var missing []missingEntry
	for rows.Next() {
		var entry missingEntry
		if err := rows.Scan(&entry.bbsID, &entry.sourceKey); err != nil {
			rows.Close()
			return err
		}
		if _, ok := seen[entry.sourceKey]; !ok {
			missing = append(missing, entry)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, entry := range missing {
		if _, err := tx.ExecContext(ctx, `UPDATE source_entry SET active=0,missing_since=CURRENT_TIMESTAMP WHERE source=? AND source_key=? AND active=1`, sourceName, entry.sourceKey); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, entry.bbsID, 0, "source_disappeared", sourceName, "presence", "present", "missing", entry.sourceKey); err != nil {
			return err
		}
	}
	return nil
}

func fillCanonicalMetadata(ctx context.Context, tx *sql.Tx, bbsID int64, e source.Entry) error {
	_, err := tx.ExecContext(ctx, `UPDATE bbs SET
 name=CASE WHEN trim(name)='' OR name='Unknown BBS' THEN ? ELSE name END,
 software=CASE WHEN trim(software)='' THEN ? ELSE software END,
 country=CASE WHEN trim(country)='' THEN ? ELSE country END,
 description=CASE WHEN trim(description)='' THEN ? ELSE description END,
 updated_at=CURRENT_TIMESTAMP WHERE id=?`, e.Name, e.Software, e.Country, e.Description, bbsID)
	return err
}

func addEvent(ctx context.Context, tx *sql.Tx, bbsID, endpointID int64, kind, sourceName, field, oldValue, newValue, detail string) error {
	var endpoint any
	if endpointID > 0 {
		endpoint = endpointID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO change_event(bbs_id,endpoint_id,kind,source,field,old_value,new_value,detail)
VALUES(?,?,?,?,?,?,?,?)`, bbsID, endpoint, kind, sourceName, field, oldValue, newValue, detail)
	return err
}
