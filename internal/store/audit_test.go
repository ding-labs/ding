package store

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/access"
	"testing"
)

func TestAuditCommitsAndRollsBackWithMutation(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := access.WithOperation(access.WithPrincipal(context.Background(), access.Principal{User: "alice", Workspace: "team", Role: access.Editor, Kind: "browser"}), "apply", "watch:build")
	// Use a temporary table so the assertion does not depend on watch schema.
	if err := s.Update(context.Background(), func(tx *Tx) error { _, err := tx.sql.Exec("CREATE TABLE audit_test(value TEXT)"); return err }); err != nil {
		t.Fatal(err)
	}
	mutate := func(tx *Tx) error {
		_, err := tx.sql.Exec("INSERT INTO audit_test VALUES('secret payload not in audit')")
		return err
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := mutate(tx); err != nil {
			return err
		}
		return fmt.Errorf("abort")
	}); err == nil {
		t.Fatal("expected rollback")
	}
	if err := s.Update(ctx, mutate); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(*Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		rows, err := tx.Audit(0)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].Actor != "alice" || rows[0].Action != "apply" || rows[0].Target != "watch:build" {
			t.Fatalf("unexpected audit: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
