package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

func openPostgresTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	migration := postgresTestPool(t, "migration")
	if err := MigratePostgres(ctx, migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	pool := postgresTestPool(t, "app")
	workspace := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano()))))
	s, err := OpenPostgres(ctx, pool, workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgresAuditRetentionAndRollback(t *testing.T) {
	s := openPostgresTestStore(t)
	if err := s.Update(context.Background(), func(tx *Tx) error {
		p := access.Principal{User: "before", Workspace: s.workspace}
		if err := tx.auditMutation(p); err != nil {
			return err
		}
		// Model an old retained row without creating ten thousand rows. Retention
		// must include the sequence allocated by the new audit insertion.
		if _, err := tx.sql.Exec("UPDATE workspaces SET audit_sequence=10001"); err != nil {
			return err
		}
		if _, err := tx.sql.Exec("SAVEPOINT audit_test"); err != nil {
			return err
		}
		p.User = "after"
		if err := tx.auditMutation(p); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		if rows, err := tx.Audit(0); err != nil || len(rows) != 1 || rows[0].Actor != "after" {
			t.Fatal("retention used stale sequence", rows, err)
		}
		if _, err := tx.sql.Exec("ROLLBACK TO SAVEPOINT audit_test"); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		if rows, err := tx.Audit(0); err != nil || len(rows) != 1 || rows[0].Actor != "before" {
			t.Fatal("audit insertion or retention survived rollback", rows, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreFacadeAuditAndPoolOwnership(t *testing.T) {
	s := openPostgresTestStore(t)
	ctx := context.Background()
	attributed := access.WithOperation(access.WithPrincipal(ctx, access.Principal{User: "alice", Workspace: s.workspace, Role: access.Editor}), "apply", "watch:health")
	if err := s.Update(attributed, func(tx *Tx) error { stateContract(t, tx); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(attributed, func(*Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		rows, err := tx.Audit(0)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].Actor != "alice" {
			t.Fatal("mutation audit mismatch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		_, err := tx.sql.Exec("DELETE FROM watches")
		if err == nil {
			t.Fatal("read-only snapshot mutated")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pool := s.db
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pool.PingContext(ctx); err != nil {
		t.Fatal("tenant close shut down shared instance pool")
	}
	if err := s.Update(ctx, func(*Tx) error { return nil }); err != ErrClosed {
		t.Fatal("closed facade accepted work", err)
	}
	reopened, err := OpenPostgres(ctx, pool, s.workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.View(ctx, func(tx *Tx) error {
		rows, err := tx.Entities("health")
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatal("reopen lost state")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
