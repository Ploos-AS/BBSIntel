package ingest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Ploos-AS/BBSIntel/internal/source"
)

func Import(ctx context.Context, db *sql.DB, adapter source.Adapter) (int, error) {
	entries, err := adapter.Fetch(ctx)
	if err != nil { return 0, err }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return 0, err }
	defer tx.Rollback()
	count := 0
	for _, e := range entries {
		if err := upsert(ctx, tx, e); err != nil { return count, err }
		count++
	}
	if err := tx.Commit(); err != nil { return count, err }
	return count, nil
}

func upsert(ctx context.Context, tx *sql.Tx, e source.Entry) error {
	var bbsID int64
	err := tx.QueryRowContext(ctx, `SELECT bbs_id FROM source_entry WHERE source=? AND source_key=?`, e.Source, e.SourceKey).Scan(&bbsID)
	if err != nil && err != sql.ErrNoRows { return err }
	if err == sql.ErrNoRows {
		res, err := tx.ExecContext(ctx, `INSERT INTO bbs(name,software,country,description) VALUES(?,?,?,?)`, e.Name, e.Software, e.Country, e.Description)
		if err != nil { return err }
		bbsID, err = res.LastInsertId(); if err != nil { return err }
		_, err = tx.ExecContext(ctx, `INSERT INTO source_entry(bbs_id,source,source_key,source_url) VALUES(?,?,?,?)`, bbsID, e.Source, e.SourceKey, e.SourceURL)
		if err != nil { return err }
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE bbs SET name=?,software=?,country=?,description=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, e.Name, e.Software, e.Country, e.Description, bbsID)
		if err != nil { return err }
		_, err = tx.ExecContext(ctx, `UPDATE source_entry SET source_url=?,last_seen=CURRENT_TIMESTAMP WHERE source=? AND source_key=?`, e.SourceURL, e.Source, e.SourceKey)
		if err != nil { return err }
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?) ON CONFLICT(protocol,hostname,port) DO UPDATE SET bbs_id=excluded.bbs_id`, bbsID, e.Protocol, e.Hostname, e.Port)
	if err != nil { return fmt.Errorf("endpoint %s:%d: %w", e.Hostname, e.Port, err) }
	return nil
}
