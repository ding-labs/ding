package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresIngestGuardHoldsRevocationUntilCommit(t *testing.T) {
	f := pgtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	db, err := OpenPostgres(ctx, f.App)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := db.Enroll(ctx, "issuer", "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.Principal(ctx, workspace.User, workspace.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := db.CreateBoundService(ctx, p, "producer", []string{"ingest"}, []string{"push"}, 5, map[string]string{"push": "lifetime"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][4]string{
		{"wrong-token", workspace.ID, "push", "lifetime"},
		{service.Token, "another-workspace", "push", "lifetime"},
		{service.Token, workspace.ID, "another-watch", "lifetime"},
		{service.Token, workspace.ID, "push", "another-lifetime"},
	} {
		tx, err := f.App.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.IngestGuard(input[0], service.ID, input[1], input[2], input[3]).Lock(ctx, tx)
		tx.Rollback()
		if !errors.Is(err, ErrServiceRevoked) {
			t.Fatal("out-of-scope producer admitted", err)
		}
	}
	// A statement that began before a permission change must recheck the grant
	// after waiting for its workspace lock, despite the combined query snapshot.
	modifier := f.NewAppPool(2)
	blocked, err := modifier.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Rollback()
	if _, err := blocked.ExecContext(ctx, "SELECT id FROM ding_control.workspaces WHERE id=$1 FOR UPDATE", workspace.ID); err != nil {
		t.Fatal(err)
	}
	waiting, err := f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer waiting.Rollback()
	var pid int
	if err := waiting.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	denied := make(chan error, 1)
	go func() {
		_, err := db.IngestGuard(service.Token, service.ID, workspace.ID, "push", "lifetime").Lock(ctx, waiting)
		denied <- err
	}()
	for {
		var locked bool
		if err := modifier.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&locked); err != nil {
			t.Fatal(err)
		}
		if locked {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := blocked.ExecContext(ctx, "UPDATE ding_control.service_grants SET watches='[]' WHERE id=$1", service.ID); err != nil {
		t.Fatal(err)
	}
	if err := blocked.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-denied; !errors.Is(err, ErrServiceRevoked) {
		t.Fatal("admitted grant from before lock wait", err)
	}
	waiting.Rollback()
	if _, err := modifier.ExecContext(ctx, `UPDATE ding_control.service_grants SET watches='["push"]' WHERE id=$1`, service.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	guard := db.IngestGuard(service.Token, service.ID, workspace.ID, "push", "lifetime")
	check, err := guard.Lock(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := OpenPostgres(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); done <- other.RevokeService(ctx, p, service.ID) }()
	<-started
	select {
	case err := <-done:
		t.Fatal("revocation crossed acceptance locks", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = check(); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	tx, err = f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = guard.Lock(ctx, tx); !errors.Is(err, ErrServiceRevoked) {
		t.Fatal("revoked producer admitted", err)
	}
}
