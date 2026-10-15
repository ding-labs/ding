package store

import (
	"context"
	"fmt"
)

func (t *Tx) postgresUsage(full bool) (Usage, error) {
	var u Usage
	query := `SELECT
 (SELECT count(*) FROM watches WHERE workspace_id=current_setting('ding.workspace') AND status!='deleted'),
 (SELECT count(*) FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND status IN ('pending','leased')),
 logical_bytes FROM workspaces WHERE workspace_id=current_setting('ding.workspace')`
	if !full {
		err := t.sql.QueryRowContext(t.ctx, query).Scan(&u.Watches, &u.Pending, &u.Bytes)
		return u, err
	}
	query = `SELECT
 (SELECT count(*) FROM watches WHERE workspace_id=current_setting('ding.workspace') AND status!='deleted'),
 (SELECT count(*) FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND status IN ('pending','leased')),
 logical_bytes,
 (SELECT count(*) FROM observations WHERE workspace_id=current_setting('ding.workspace')),
 (SELECT count(*) FROM events WHERE workspace_id=current_setting('ding.workspace')),
 (SELECT count(*) FROM entities WHERE workspace_id=current_setting('ding.workspace')),
 (SELECT count(*) FROM timers WHERE workspace_id=current_setting('ding.workspace'))
 FROM workspaces WHERE workspace_id=current_setting('ding.workspace')`
	err := t.sql.QueryRowContext(t.ctx, query).Scan(&u.Watches, &u.Pending, &u.Bytes, &u.Observations, &u.Events, &u.Entities, &u.Timers)
	return u, err
}

func (s *Store) limitPostgresBytes(ctx context.Context, max int64) error {
	if max < 1 {
		return fmt.Errorf("workspace logical byte limit must be positive")
	}
	return s.updatePostgres(ctx, func(tx *Tx) error {
		result, err := tx.sql.ExecContext(ctx, "UPDATE workspaces SET max_logical_bytes=$1 WHERE workspace_id=$2 AND logical_bytes<=$1", max, s.workspace)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrQuota
		}
		return nil
	})
}

func (s *Store) healthPostgres(ctx context.Context) (Health, error) {
	h := Health{Backend: "postgres", Schema: PostgresSchemaVersion, Journal: "wal"}
	err := s.View(ctx, func(tx *Tx) error {
		var fsync, fullPage, commit string
		if err := tx.sql.QueryRowContext(ctx, `SELECT current_setting('server_version'),current_setting('fsync'),current_setting('full_page_writes'),current_setting('synchronous_commit'),logical_bytes FROM workspaces WHERE workspace_id=$1`, s.workspace).Scan(&h.Postgres, &fsync, &fullPage, &commit, &h.LogicalBytes); err != nil {
			return err
		}
		if fsync != "on" || fullPage != "on" || commit != "on" {
			return fmt.Errorf("PostgreSQL requires fsync, full_page_writes and synchronous_commit on")
		}
		// This confirms runtime readiness and durability settings. It does not claim
		// a SQLite-style physical integrity scan of the remote database files.
		h.Integrity = "ok"
		h.Durability = "fsync=on,full_page_writes=on,synchronous_commit=on"
		return nil
	})
	return h, err
}
