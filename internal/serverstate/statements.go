package state

import (
	"context"
	"database/sql"
)

// Statements carry explicit backend SQL; control metadata never rewrites SQL
// placeholders or depends on a pooled connection's previous tenant search path.
type statement struct{ sqlite, postgres string }
type transaction struct {
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
	return &transaction{Tx: tx, postgres: d.postgres}, nil
}

func (d *DB) exec(ctx context.Context, q statement, args ...any) (sql.Result, error) {
	return d.sql.ExecContext(ctx, d.query(q), args...)
}
