package cloudbackup

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestEncryptedBackupFreshRestoreAndQuarantine(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	account, _ := db.Enroll(ctx, "issuer", "immutable", 1)
	session, _ := db.NewSession(ctx, account.ID, "device", time.Now())
	key := make([]byte, 32)
	rand.Read(key)
	vault, _ := state.NewVault(db, key)
	_ = vault.Put(ctx, account.ID, "WEBHOOK", "https://example.com/synthetic-secret")
	identity, _ := age.GenerateX25519Identity()
	archive := filepath.Join(t.TempDir(), "backup.age")
	if err := Write(ctx, dir, archive, identity.Recipient().String()); err == nil {
		t.Fatal("backup did not fence running service")
	}
	tenant, err := store.Open(ctx, filepath.Join(dir, "workspaces", account.ID))
	if err != nil {
		t.Fatal(err)
	}
	app := watchrun.New(tenant)
	_, err = app.Apply(ctx, watchrun.ApplyRequest{Manifest: `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: hook}
spec: {type: webhook, urlRef: {env: WEBHOOK}}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api}
spec:
 source: {type: http, url: https://example.com, every: 5m}
 condition: {field: http.status, operator: gte, value: 500}
 policy: {consecutive: 1, recoverAfter: 1}
 destinations: [{ref: hook, events: [firing]}]
`})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := app.Record(ctx, "api")
	_, err = app.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 500}}}}, "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tenant.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, dir, archive, identity.Recipient().String()); err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, dir, archive, identity.Recipient().String()); err == nil {
		t.Fatal("backup overwritten")
	}
	encrypted, _ := os.ReadFile(archive)
	if strings.Contains(string(encrypted), "synthetic-secret") || strings.Contains(string(encrypted), "SQLite format") {
		t.Fatal("plaintext leaked")
	}
	restore := filepath.Join(t.TempDir(), "restored")
	if err := Restore(ctx, archive, restore, identity.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restore, Quarantine)); err != nil {
		t.Fatal("missing quarantine")
	}
	restored, err := state.Open(ctx, restore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Session(ctx, session.Token, time.Now()); err == nil {
		t.Fatal("old session resurrected")
	}
	v, _ := state.NewVault(restored, key)
	secret, err := v.Get(ctx, account.ID, "WEBHOOK")
	if err != nil || !strings.Contains(secret, "synthetic-secret") {
		t.Fatal("credentials cannot recover with separate master key", err)
	}
	_ = restored.Close()
	tenant, err = store.Open(ctx, filepath.Join(restore, "workspaces", account.ID))
	if err != nil {
		t.Fatal(err)
	}
	var pending int
	var events []watch.Event
	err = tenant.View(ctx, func(tx *store.Tx) error {
		r, e := tx.Watch("api")
		if e != nil {
			return e
		}
		if r.Status != "paused" {
			t.Fatal("restored producer running")
		}
		pending, e = tx.Pending()
		if e != nil {
			return e
		}
		events, e = tx.Events("api", 0, 100)
		return e
	})
	if err != nil || pending != 0 || len(events) < 2 {
		t.Fatal("quarantine lost history or retained external effects", pending, len(events), err)
	}
	_ = tenant.Close()
	if err := Release(ctx, restore); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restore, Quarantine)); !os.IsNotExist(err) {
		t.Fatal("quarantine was not released")
	}
	truncated := filepath.Join(t.TempDir(), "truncated.age")
	_ = os.WriteFile(truncated, encrypted[:len(encrypted)-8], 0600)
	bad := filepath.Join(t.TempDir(), "bad")
	if err := Restore(ctx, truncated, bad, identity.String()); err == nil {
		t.Fatal("truncated ciphertext accepted")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("failed restore was published")
	}
}
