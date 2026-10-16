package state

import (
	"context"
	"time"
)

// Receipts for external effects deliberately do not expire: dropping their keys
// could turn an old retry into a fresh delivery. Their explicit quotas bound them.
func (d *DB) Collect(ctx context.Context, now time.Time) error {
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{tx.query(qCollect1), tx.query(qCollect2), tx.query(qCollect3)} {
		if _, err := tx.ExecContext(ctx, query, now.Unix()); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, tx.query(qCollect4), now.Add(-time.Hour).Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.query(qCollect5), now.AddDate(0, -3, 0).UTC().Format("2006-01")); err != nil {
		return err
	}
	return tx.Commit()
}
