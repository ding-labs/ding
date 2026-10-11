package state

import (
	"context"
	"testing"
)

func TestEnrollmentIsDurableBoundedAndSingleOwner(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(ctx, dir); err == nil {
		other.Close()
		t.Fatal("two cloud owners admitted")
	}
	a, err := db.Enroll(ctx, "https://issuer.example", "immutable-id", 1)
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.Enroll(ctx, "https://issuer.example", "immutable-id", 1)
	if err != nil || again.ID != a.ID {
		t.Fatal("identity did not reuse workspace")
	}
	if _, err := db.Enroll(ctx, "https://other-issuer.example", "immutable-id", 1); err == nil {
		t.Fatal("enrollment cap or issuer binding failed")
	}
	if _, err := db.Enroll(ctx, "https://issuer.example", "new-id", 0); err == nil {
		t.Fatal("closed enrollment created an account")
	}
	if again, err := db.Enroll(ctx, "https://issuer.example", "immutable-id", 0); err != nil || again.ID != a.ID {
		t.Fatal("closed enrollment locked out an existing user", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.Account(ctx, a.ID)
	if err != nil || got.Subject != a.Subject {
		t.Fatal("identity did not survive restart", err)
	}
}
