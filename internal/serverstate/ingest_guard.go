package state

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/ding-labs/ding/internal/database"
)

var ErrServiceRevoked = database.ErrAuthorization

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
		// Each materialized stage depends on the preceding locked row. Preserve
		// lifecycle lock order in one round trip; permission decisions stay in Go.
		var expiry int64
		grant, err := scanService(tx.QueryRowContext(ctx, `WITH scope AS MATERIALIZED (
 SELECT id FROM ding_control.workspaces WHERE id=$1 AND deleting=false FOR SHARE),
 grant_row AS MATERIALIZED (
 SELECT g.* FROM scope s JOIN ding_control.service_grants g ON g.workspace=s.id
 WHERE g.id=$2 FOR SHARE OF g)
 SELECT g.id,g.name,g.workspace,g.created_by,g.scopes,g.watches,g.expires_at,g.lifetimes,k.expires_at
 FROM grant_row g JOIN ding_control.service_keys k ON k.service=g.id
 WHERE k.hash=$3 FOR SHARE OF k`, workspace, producer, TokenHash(token)), &expiry)
		if err != nil {
			return nil, unavailable(err)
		}
		if !slices.Contains(grant.Scopes, "ingest") || !slices.Contains(grant.Watches, watch) || grant.Lifetimes[watch] != lifetime {
			return nil, ErrServiceRevoked
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

// WebhookIngestGuard reuses the provider's existing Go verification while its
// connection and workspace rows are locked. Ciphertext reads stay in this tx.
func (v *Vault) WebhookIngestGuard(id, workspace string, authorize func(context.Context) error, until func() time.Time) database.TransactionGuard {
	if !v.db.postgres {
		return database.TransactionGuard{}
	}
	return database.TransactionGuard{Pool: v.db.sql, Lock: func(ctx context.Context, tx *sql.Tx) (func() error, error) {
		var found string
		if err := tx.QueryRowContext(ctx, "SELECT id FROM ding_control.workspaces WHERE id=$1 AND deleting=false FOR SHARE", workspace).Scan(&found); err != nil {
			return nil, ErrServiceRevoked
		}
		if err := tx.QueryRowContext(ctx, "SELECT id FROM ding_control.webhook_connections WHERE id=$1 AND workspace=$2 FOR SHARE", id, workspace).Scan(&found); err != nil {
			return nil, ErrServiceRevoked
		}
		if err := authorize(database.WithTransaction(ctx, v.db.sql, tx)); err != nil {
			return nil, ErrServiceRevoked
		}
		deadline := until()
		check := func() error {
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				return ErrServiceRevoked
			}
			return nil
		}
		return check, check()
	}}
}
