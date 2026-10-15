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
