package state

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/ding-labs/ding/internal/database"
)

var ErrServiceRevoked = errors.New("producer credential expired or unavailable")

func (t *transaction) lockService(ctx context.Context, workspace, id string) error {
	if !t.postgres {
		return nil
	}
	var found string
	err := t.QueryRowContext(ctx, "SELECT id FROM ding_control.service_grants WHERE workspace=$1 AND id=$2 FOR UPDATE", workspace, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// IngestGuard locks workspace -> grant -> key, matching lifecycle mutations.
// All reads use the acceptance transaction, including with a one-slot pool.
func (d *DB) IngestGuard(token, producer, workspace, watch, lifetime string) database.TransactionGuard {
	if !d.postgres {
		return database.TransactionGuard{}
	}
	return database.TransactionGuard{Pool: d.sql, Lock: func(ctx context.Context, tx *sql.Tx) (func() error, error) {
		unavailable := func(err error) error {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrServiceRevoked
			}
			return err
		}
		var scope string
		if err := tx.QueryRowContext(ctx, "SELECT id FROM ding_control.workspaces WHERE id=$1 AND deleting=false FOR SHARE", workspace).Scan(&scope); err != nil {
			return nil, unavailable(err)
		}
		grant, err := scanService(tx.QueryRowContext(ctx, "SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM ding_control.service_grants WHERE id=$1 AND workspace=$2 FOR SHARE", producer, workspace))
		if err != nil {
			return nil, unavailable(err)
		}
		if !slices.Contains(grant.Scopes, "ingest") || grant.Lifetimes[watch] != lifetime {
			return nil, ErrServiceRevoked
		}
		var expiry int64
		if err = tx.QueryRowContext(ctx, "SELECT expires_at FROM ding_control.service_keys WHERE service=$1 AND hash=$2 FOR SHARE", producer, TokenHash(token)).Scan(&expiry); err != nil {
			return nil, unavailable(err)
		}
		check := func() error {
			if !time.Now().Before(time.Unix(expiry, 0)) {
				return ErrServiceRevoked
			}
			return nil
		}
		if err = check(); err != nil {
			return nil, err
		}
		return check, nil
	}}
}
