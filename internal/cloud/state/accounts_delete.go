package state

import (
	"context"
	"fmt"
)

func (d *DB) BeginAccountDelete(ctx context.Context, id string) error {
	r, err := d.sql.ExecContext(ctx, "UPDATE accounts SET deleting=1 WHERE id=?", id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("account unavailable")
	}
	return nil
}
func (d *DB) DeletingAccounts(ctx context.Context) ([]string, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT id FROM accounts WHERE deleting=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (d *DB) FinishAccountDelete(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM accounts WHERE id=? AND deleting=1", id)
	return err
}
