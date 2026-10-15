package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/database"
)

var postgresWorkspaceID = regexp.MustCompile(`^[a-f0-9]{64}$`)

// OpenPostgres borrows the instance pool. Closing this tenant facade does not
// close that pool. Migration and instance ownership belong to the server root.
func OpenPostgres(ctx context.Context, db *sql.DB, workspace string, owners ...*database.Ownership) (*Store, error) {
	if db == nil || !postgresWorkspaceID.MatchString(workspace) {
		return nil, fmt.Errorf("invalid PostgreSQL workspace")
	}
	if err := ValidatePostgres(ctx, db); err != nil {
		return nil, err
	}
	s := &Store{db: db, postgres: true, workspace: workspace, writer: make(chan struct{}, 1)}
	if len(owners) > 0 {
		s.ownership = owners[0]
	}
	tx, err := s.beginPostgres(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	created, err := tx.ExecContext(ctx, "INSERT INTO workspaces(workspace_id) VALUES($1) ON CONFLICT DO NOTHING", workspace)
	if err != nil {
		return nil, err
	}
	inserted, err := created.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted == 1 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO push_usage(singleton) VALUES(1)"); err != nil {
			return nil, err
		}
	} else {
		var n int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM push_usage WHERE singleton=1").Scan(&n); err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, fmt.Errorf("workspace durable usage is missing; restore or reconcile before startup")
		}
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO metadata(key,value) VALUES('store_id',$1) ON CONFLICT DO NOTHING", hex.EncodeToString(id[:])); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest)
 SELECT 1,count(*),coalesce(sum(CASE WHEN expires_at<=0 THEN 1 ELSE 0 END),0),0,coalesce(min(expires_at),0)
 FROM push_receipts WHERE NOT EXISTS(SELECT 1 FROM push_receipt_stats WHERE singleton=1)
 ON CONFLICT DO NOTHING`); err != nil {
		return nil, err
	}

	if err = s.ownership.Fence(ctx, tx, true); err != nil {
		return nil, err
	}
	return s, tx.Commit()
}

func (s *Store) beginPostgres(ctx context.Context, readOnly bool) (*sql.Tx, error) {
	isolation := sql.LevelReadCommitted
	if readOnly {
		isolation = sql.LevelRepeatableRead
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly, Isolation: isolation})
	if err != nil {
		return nil, err
	}
	if err = s.ownership.Fence(ctx, tx, !readOnly); err != nil {
		tx.Rollback()
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "SET LOCAL search_path=ding_runtime,pg_catalog"); err == nil {
		_, err = tx.ExecContext(ctx, "SELECT set_config('ding.workspace',$1,true)", s.workspace)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *Store) updatePostgres(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.beginPostgres(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	check, err := database.LockTransactionGuard(ctx, tx, s.db)
	if err != nil {
		return err
	}
	// One database row orders workspace evaluation, range allocation and quota
	// accounting, including callers in another process. Receiver I/O stays outside.
	var scope string
	if err = tx.QueryRowContext(ctx, "SELECT workspace_id FROM workspaces WHERE workspace_id=$1 FOR UPDATE", s.workspace).Scan(&scope); err != nil {
		return err
	}
	stx := &Tx{ctx: ctx, sql: tx, postgres: true, writable: true}
	if err = fn(stx); err != nil {
		return err
	}
	if p, ok := access.FromContext(ctx); ok && stx.changed {
		if p.Workspace != s.workspace {
			return access.ErrDenied
		}
		if err = stx.auditMutation(p); err != nil {
			return err
		}
	}
	var bytes, limit int64
	if err = tx.QueryRowContext(ctx, "SELECT logical_bytes,max_logical_bytes FROM workspaces WHERE workspace_id=$1", s.workspace).Scan(&bytes, &limit); err != nil {
		return err
	}
	if limit > 0 && bytes > limit {
		return ErrQuota
	}
	if err = check(); err != nil {
		return err
	}
	if err = s.ownership.Fence(ctx, tx, true); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) viewPostgres(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.beginPostgres(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&Tx{ctx: ctx, sql: tx, postgres: true}); err != nil {
		return err
	}
	return s.ownership.Fence(ctx, tx, false)
}

// ValidatePostgres checks readiness before a server can enroll its first workspace.
func ValidatePostgres(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM ding_meta.versions WHERE component='runtime'").Scan(&version); err != nil || version != PostgresSchemaVersion {
		return fmt.Errorf("PostgreSQL runtime schema is unavailable or incompatible; run server migration")
	}
	var unsafe bool
	err := db.QueryRowContext(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR pg_has_role(current_user,n.nspowner,'MEMBER') OR EXISTS(SELECT 1 FROM pg_class c WHERE c.relnamespace=n.oid AND c.relkind='r' AND (NOT c.relrowsecurity OR NOT c.relforcerowsecurity)) FROM pg_roles r JOIN pg_namespace n ON n.nspname='ding_runtime' WHERE r.rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		return fmt.Errorf("PostgreSQL runtime requires a non-owner application role and forced workspace isolation")
	}

	return nil
}
