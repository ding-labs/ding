package database

import (
	"context"
	"database/sql"
	"fmt"
)

func Quarantined(ctx context.Context, pool *sql.DB) (bool, error) {
	var value bool
	err := pool.QueryRowContext(ctx, "SELECT quarantined FROM ding_meta.lifecycle WHERE singleton=1").Scan(&value)
	if err != nil {
		return false, SafeError("read restore quarantine", err)
	}
	return value, nil
}

// SetQuarantine is independent of restored domain schemas, so an interrupted
// native restore cannot erase the flag and allow another server to start.
func SetQuarantine(ctx context.Context, pool *sql.DB, owner *Ownership, value bool) error {
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = owner.Fence(ctx, tx, true); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE ding_meta.lifecycle SET quarantined=$1 WHERE singleton=1", value)
	if err != nil {
		return SafeError("write restore quarantine", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("restore quarantine metadata is missing")
	}
	if err = owner.Fence(ctx, tx, true); err != nil {
		return err
	}
	return tx.Commit()
}
