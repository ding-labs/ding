package state

import (
	"context"
	"database/sql"
	"github.com/ding-labs/ding/internal/database"
)

// Statements carry explicit backend SQL; control metadata never rewrites SQL
// placeholders or depends on a pooled connection's previous tenant search path.
type statement struct{ sqlite, postgres string }
type transaction struct {
	ctx       context.Context
	ownership *database.Ownership
	*sql.Tx
	postgres bool
}

func (d *DB) query(q statement) string {
	if d.postgres {
		return q.postgres
	}
	return q.sqlite
}
func (t *transaction) query(q statement) string {
	if t.postgres {
		return q.postgres
	}
	return q.sqlite
}

func (d *DB) begin(ctx context.Context) (*transaction, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err = d.ownership.Fence(ctx, tx, true); err != nil {
		tx.Rollback()
		return nil, err
	}
	if d.postgres {
		var singleton int
		if err = tx.QueryRowContext(ctx, "SELECT singleton FROM ding_control.writer WHERE singleton=1 FOR UPDATE").Scan(&singleton); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	return &transaction{Tx: tx, postgres: d.postgres, ctx: ctx, ownership: d.ownership}, nil
}

func (d *DB) exec(ctx context.Context, q statement, args ...any) (sql.Result, error) {
	if !d.postgres {
		return d.sql.ExecContext(ctx, q.sqlite, args...)
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, q.postgres, args...)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (t *transaction) Commit() error {
	if err := t.ownership.Fence(t.ctx, t.Tx, true); err != nil {
		return err
	}
	return t.Tx.Commit()
}
