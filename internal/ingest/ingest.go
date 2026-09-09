package ingest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Ploos-AS/BBSIntel/internal/source"
)

func Import(ctx context.Context, db *sql.DB, adapter source.Adapter) (int, error) {
	entries, err := adapter.Fetch(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	for _, e := range entries {
		if err := upsert(ctx, tx, e); err != nil {
			return count, err
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func upsert(ctx context.Context, tx *sql.Tx, e source.Entry) error {
	var bbsID int64
	err := tx.QueryRowContext(ctx, `SELECT bbs_id FROM source_entry WHERE source=? AND source_key=?`, e.Source, e.SourceKey).Scan(&bbsID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if err == sql.ErrNoRows {
		// A second directory may describe an endpoint already known from another
		// source. Reuse that BBS identity instead of moving the endpoint to a new
		// BBS row and leaving the previous identity orphaned.
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
		} else {
			// Preserve the existing canonical metadata while filling blanks from the
			// newly discovered source. Source-specific provenance remains represented
			// by source_entry and can be expanded independently later.
			_, err = tx.ExecContext(ctx, `UPDATE bbs SET
 name=CASE WHEN trim(name)='' OR name='Unknown BBS' THEN ? ELSE name END,
 software=CASE WHEN trim(software)='' THEN ? ELSE software END,
 country=CASE WHEN trim(country)='' THEN ? ELSE country END,
 description=CASE WHEN trim(description)='' THEN ? ELSE description END,
 updated_at=CURRENT_TIMESTAMP WHERE id=?`, e.Name, e.Software, e.Country, e.Description, bbsID)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO source_entry(bbs_id,source,source_key,source_url) VALUES(?,?,?,?)`, bbsID, e.Source, e.SourceKey, e.SourceURL)
		if err != nil {
			return err
		}
	} else {
		// Refresh source presence and only fill canonical metadata that is still
		// missing. With multiple sources, the last importer must not clobber the
		// metadata supplied by an earlier source.
		_, err = tx.ExecContext(ctx, `UPDATE bbs SET
 name=CASE WHEN trim(name)='' OR name='Unknown BBS' THEN ? ELSE name END,
 software=CASE WHEN trim(software)='' THEN ? ELSE software END,
 country=CASE WHEN trim(country)='' THEN ? ELSE country END,
 description=CASE WHEN trim(description)='' THEN ? ELSE description END,
 updated_at=CURRENT_TIMESTAMP WHERE id=?`, e.Name, e.Software, e.Country, e.Description, bbsID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE source_entry SET source_url=?,last_seen=CURRENT_TIMESTAMP WHERE source=? AND source_key=?`, e.SourceURL, e.Source, e.SourceKey)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?) ON CONFLICT(protocol,hostname,port) DO NOTHING`, bbsID, e.Protocol, e.Hostname, e.Port)
	if err != nil {
		return fmt.Errorf("endpoint %s:%d: %w", e.Hostname, e.Port, err)
	}
	return nil
}
