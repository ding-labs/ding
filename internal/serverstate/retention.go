package state

import (
	"context"
	"time"
)

// Receipts for external effects deliberately do not expire: dropping their keys
// could turn an old retry into a fresh delivery. Their explicit quotas bound them.
func (d *DB) Collect(ctx context.Context, now time.Time) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"sessions", "logins", "devices"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE expires_at<=?", now.Unix()); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM reservations WHERE created_at<?", now.Add(-time.Hour).Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM usage WHERE period<?", now.AddDate(0, -3, 0).UTC().Format("2006-01")); err != nil {
		return err
	}
	return tx.Commit()
}
