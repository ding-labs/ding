package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDestinationReadinessBindingsAndExpiry(t *testing.T) {
	db, err := Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testDestinationReadiness(t, db)
}
func TestPostgresDestinationReadinessBindingsAndExpiry(t *testing.T) {
	testDestinationReadiness(t, openPostgresTestStore(t))
}
func testDestinationReadiness(t *testing.T, db *Store) {
	now := time.Now().UTC()
	binding := DestinationSetup{Account: "owner", Workspace: "workspace", Destination: "alerts", Revision: "revision", Digest: strings.Repeat("a", 64)}
	key, digest := "readiness-operation-0001", strings.Repeat("b", 64)
	update := func(fn func(*Tx) error) {
		t.Helper()
		if err := db.Update(t.Context(), fn); err != nil {
			t.Fatal(err)
		}
	}
	check := func(setup DestinationSetup, at time.Time, want string) {
		t.Helper()
		if err := db.View(t.Context(), func(tx *Tx) error {
			got, err := tx.DestinationReadiness(setup, at)
			if err == nil && got.State != want {
				t.Errorf("state %s, want %s", got.State, want)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), digest) || strings.Contains(string(raw), binding.Digest) {
				t.Error("private binding exposed")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	check(binding, now, "untested")
	update(func(tx *Tx) error {
		_, start, err := tx.BeginSetupProbe(key, digest, binding, now)
		if !start {
			t.Error("no reservation")
		}
		return err
	})
	check(binding, now, "pending")
	if err := db.Update(t.Context(), func(tx *Tx) error { _, err := tx.ConfirmDestinationProbe(key, binding, now); return err }); !errors.Is(err, ErrConflict) {
		t.Fatal("pending accepted", err)
	}
	update(func(tx *Tx) error { return tx.FinishDestinationProbe(key, "accepted") })
	check(binding, now, "accepted")
	for range 2 {
		update(func(tx *Tx) error {
			_, err := tx.ConfirmDestinationProbe(key, binding, now.Add(time.Minute))
			return err
		})
	}
	check(binding, now.Add(time.Hour), "confirmed")
	other := binding
	other.Account = "other"
	check(other, now, "untested")
	other = binding
	other.Workspace = "other"
	check(other, now, "untested")
	for _, field := range []string{"revision", "credential"} {
		changed := binding
		if field == "revision" {
			changed.Revision = "new"
		} else {
			changed.Digest = strings.Repeat("c", 64)
		}
		check(changed, now, "stale")
		if err := db.Update(t.Context(), func(tx *Tx) error { _, err := tx.ConfirmDestinationProbe(key, changed, now); return err }); !errors.Is(err, ErrConflict) {
			t.Fatal("stale accepted", err)
		}
	}
	check(binding, now.Add(DestinationConfirmationTTL), "expired")
	if err := db.Update(t.Context(), func(tx *Tx) error {
		_, err := tx.ConfirmDestinationProbe(key, binding, now.Add(DestinationConfirmationTTL))
		return err
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("expired accepted", err)
	}
	// Expiry must never remove the durable no-resend receipt.
	update(func(tx *Tx) error {
		p, start, err := tx.BeginSetupProbe(key, digest, binding, now.Add(48*time.Hour))
		if start || p.Outcome != "accepted" {
			t.Error("expired receipt resent", p, start)
		}
		return err
	})
	update(func(tx *Tx) error {
		_, _, err := tx.BeginSetupProbe("readiness-operation-0002", digest, binding, now.Add(2*time.Minute))
		return err
	})
	check(binding, now.Add(3*time.Minute), "pending")
	if err := db.Update(t.Context(), func(tx *Tx) error {
		_, err := tx.ConfirmDestinationProbe(key, binding, now.Add(3*time.Minute))
		return err
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("old receipt confirmed", err)
	}
}
