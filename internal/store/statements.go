package store

import "database/sql"

// statement keeps dialect decisions inside data access. SQL is explicit and
// reviewed for each backend; no runtime placeholder or syntax rewriting occurs.
type statement struct{ sqlite, postgres string }

func (t *Tx) query(q statement) string {
	if t.postgres {
		return q.postgres
	}
	return q.sqlite
}

func (t *Tx) exec(q statement, args ...any) (sql.Result, error) {
	result, err := t.sql.ExecContext(t.ctx, t.query(q), args...)
	if err == nil {
		n, e := result.RowsAffected()
		if e != nil {
			return nil, e
		}
		t.changed = t.changed || n > 0
	}
	return result, err
}

// insertID preserves SQLite's returned ID and PostgreSQL's explicit RETURNING.
// PostgreSQL IDs are allocated transactionally by the operation's SQL, rather
// than an advancing global sequence that could interleave tenant receipt ranges.
func (t *Tx) insertID(q statement, args ...any) (int64, error) {
	if t.postgres {
		var id int64
		err := t.sql.QueryRowContext(t.ctx, q.postgres, args...).Scan(&id)
		if err == nil {
			t.changed = true
		}
		return id, err
	}
	result, err := t.exec(q, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
