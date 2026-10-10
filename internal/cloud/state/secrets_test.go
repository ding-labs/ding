package state

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
)

func TestVaultBindsCiphertextToWorkspaceAndName(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "https://issuer", "a", 2)
	b, _ := db.Enroll(ctx, "https://issuer", "b", 2)
	key := make([]byte, 32)
	rand.Read(key)
	vault, err := NewVault(db, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Put(ctx, a.ID, "TOKEN", "synthetic-alpha"); err != nil {
		t.Fatal(err)
	}
	if err := vault.Put(ctx, b.ID, "TOKEN", "synthetic-beta"); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{a.ID: "synthetic-alpha", b.ID: "synthetic-beta"} {
		if got, err := vault.Get(ctx, id, "TOKEN"); err != nil || got != want {
			t.Fatal("isolated lookup failed", err)
		}
	}
	var encrypted []byte
	if err := db.sql.QueryRow("SELECT ciphertext FROM secrets WHERE account=?", a.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("synthetic-alpha")) {
		t.Fatal("plaintext stored")
	}
	if _, err := db.sql.Exec("UPDATE secrets SET ciphertext=? WHERE account=?", encrypted, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get(ctx, b.ID, "TOKEN"); err == nil {
		t.Fatal("cross-workspace ciphertext accepted")
	}
	if _, err := db.sql.Exec("UPDATE secrets SET name='OTHER' WHERE account=?", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get(ctx, a.ID, "OTHER"); err == nil {
		t.Fatal("renamed ciphertext accepted")
	}
	t.Setenv("TOKEN", "process-global-must-not-leak")
	if _, ok := vault.Lookup(a.ID)("TOKEN"); ok {
		t.Fatal("fell back to process-global secret")
	}
}
