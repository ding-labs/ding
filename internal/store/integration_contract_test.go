package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func integrationContract(t *testing.T, tx *Tx) {
	t.Helper()
	c, _ := fixture(t)
	if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	g := IntegrationGrant{ID: "same-grant", Scopes: []string{"read"}, ExpiresAt: testNow.Add(time.Hour)}
	if err := tx.CreateIntegrationGrant(g, "same-hash"); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.IntegrationGrantByHash("same-hash"); err != nil || got.ID != g.ID {
		t.Fatal("grant identity mismatch", err)
	}
	preview := IntegrationPreview{ID: "same-preview", GrantID: g.ID, Manifest: "digest", Review: []byte(`{"allow":true}`), ExpiresAt: testNow.Add(time.Hour)}
	if err := tx.SaveIntegrationPreview(preview, testNow); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.IntegrationPreview(g.ID, preview.ID, testNow); err != nil || string(got.Review) != string(preview.Review) {
		t.Fatal("preview mismatch", err)
	}
	op := IntegrationOperation{Key: "same-key", Action: "apply", Digest: "digest", Result: []byte(`{"accepted":true}`), CreatedAt: testNow}
	if err := tx.SaveIntegrationOperation(g.ID, op); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.IntegrationOperation(g.ID, op.Key); err != nil || got.Digest != op.Digest {
		t.Fatal("operation mismatch", err)
	}
	if err := tx.RevokeIntegrationGrant(g.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.IntegrationGrant(g.ID); err != nil || !got.Revoked {
		t.Fatal("revocation missing", err)
	}
	if all, err := tx.IntegrationGrants(); err != nil || len(all) != 1 || !all[0].Revoked {
		t.Fatal("grant listing mismatch", err)
	}
	h := Handoff{ID: strings.Repeat("a", 64), WatchID: "health", Held: true, Destinations: map[string]string{}}
	if err := tx.SaveHandoff(h); err != nil {
		t.Fatal(err)
	}
	if hold, err := tx.HandoffHold("health"); err != nil || hold != h.ID {
		t.Fatal("transfer hold mismatch", err)
	}
	h.Held = false
	if err := tx.SaveHandoff(h); err != nil {
		t.Fatal(err)
	}
	if hold, err := tx.HandoffHold("health"); err != nil || hold != "" {
		t.Fatal("transfer release mismatch", err)
	}
	if all, err := tx.Handoffs(); err != nil || len(all) != 1 || all[0].Held {
		t.Fatal("transfer listing mismatch", err)
	}
}
func TestIntegrationContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { integrationContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		for _, suffix := range []string{"a", "b"} {
			tx := postgresScopedTx(t, db, fmt.Sprintf("integration-%d-%s", time.Now().UnixNano(), suffix))
			integrationContract(t, tx)
			if err := tx.sql.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
