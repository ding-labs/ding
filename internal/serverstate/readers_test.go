package state

import (
	"context"
	"testing"
	"time"
)

func TestCredentialReaderDoesNotWaitForWriterAndSeesCommittedRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Enroll(ctx, "issuer", "reader-test", 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := db.CreateService(ctx, p, "original", []string{"inspect"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE service_grants SET name='committed' WHERE id=?", grant.ID); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	before, err := db.ServiceToken(readCtx, grant.Token)
	if err != nil || before.Name != "original" {
		t.Fatal("reader waited or saw uncommitted value", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	after, err := db.ServiceToken(ctx, grant.Token)
	if err != nil || after.Name != "committed" {
		t.Fatal("reader reused a stale snapshot", err)
	}
	if _, err := db.readers.Exec("UPDATE service_grants SET name='forbidden'"); err == nil {
		t.Fatal("credential connection permitted writes")
	}
	if err := db.RevokeService(ctx, p, grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ServiceToken(ctx, grant.Token); err == nil {
		t.Fatal("committed revocation was invisible")
	}
}
