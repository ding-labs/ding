package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/watch"
)

func TestCursorResumeScopeExpiryAndReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	seed(t, s)
	p, _ := fixture(t)
	appendEvent := func(id string) {
		t.Helper()
		if err := s.Update(ctx, func(tx *Tx) error {
			_, err := tx.AppendEvent(watch.Event{ID: id, WatchID: "health", Revision: p.Revision, At: testNow, Type: "firing"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent("one")
	appendEvent("two")
	appendEvent("three")
	var first, second, empty EventPage
	if err := s.View(ctx, func(tx *Tx) error { var err error; first, err = tx.EventPage("health", "", 1); return err }); err != nil {
		t.Fatal(err)
	}
	if !first.More || len(first.Events) != 1 || first.Events[0].ID != "one" {
		t.Fatal(first)
	}
	s.Close()
	s = openTest(t, dir)
	if err := s.View(ctx, func(tx *Tx) error { var err error; second, err = tx.EventPage("health", first.Cursor, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if second.More || len(second.Events) != 2 || second.Events[0].ID != "two" {
		t.Fatal(second)
	}
	s.View(ctx, func(tx *Tx) error { var err error; empty, err = tx.EventPage("health", second.Cursor, 100); return err })
	if len(empty.Events) != 0 || empty.Cursor != second.Cursor {
		t.Fatal(empty)
	}
	s.View(ctx, func(tx *Tx) error {
		for _, c := range []string{"bad", base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"after":-1}`))} {
			if _, err := tx.EventPage("health", c, 100); !errors.Is(err, ErrCursor) {
				t.Fatal(err)
			}
		}
		if _, err := tx.EventPage("other", first.Cursor, 100); !errors.Is(err, ErrCursor) {
			t.Fatal("scope accepted", err)
		}
		return nil
	})
	// Non-contiguous retention keeps an older pinned event but must still expose
	// the missing later event to a reader that has not consumed it.
	if err := s.Update(ctx, func(tx *Tx) error { _, err := tx.sql.ExecContext(ctx, "DELETE FROM events WHERE id='two'"); return err }); err != nil {
		t.Fatal(err)
	}
	s.View(ctx, func(tx *Tx) error {
		if _, err := tx.EventPage("health", first.Cursor, 100); !errors.Is(err, ErrCursorExpired) {
			t.Fatal("silent gap", err)
		}
		if _, err := tx.EventPage("health", second.Cursor, 100); err != nil {
			t.Fatal("already consumed deletion", err)
		}
		page, err := tx.EventPage("health", "", 100)
		if err != nil || len(page.Events) != 2 {
			t.Fatal(page, err)
		}
		return nil
	})
	other := openTest(t, t.TempDir())
	other.View(ctx, func(tx *Tx) error {
		if _, err := tx.EventPage("health", first.Cursor, 100); !errors.Is(err, ErrCursor) {
			t.Fatal("cross-store cursor", err)
		}
		return nil
	})
}
func TestSchemaOneUpgradePreservesDataAndBackup(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(initialSchema + "; INSERT INTO metadata(key,value) VALUES('sentinel','present'); PRAGMA user_version=1;"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s := openTest(t, dir)
	var value string
	if err := s.db.QueryRow("SELECT value FROM metadata WHERE key='sentinel'").Scan(&value); err != nil || value != "present" {
		t.Fatal(value, err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "before-schema-2-*.db"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	b, err := sql.Open("sqlite", readOnlyURI(backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var version int
	b.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 1 {
		t.Fatal(version)
	}
}
func TestRetryPreservesIdentityHistoryAndProviderDeadline(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	p, d := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error { return transition(tx, p, d, "1", testNow) }); err != nil {
		t.Fatal(err)
	}
	i, err := s.Claim(ctx, testNow, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, *i, delivery.Result{Outcome: delivery.Exhausted, RetryAt: testNow.Add(time.Hour)}, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error { return tx.Retry(i.ID, testNow.Add(time.Minute)) }); err != nil {
		t.Fatal(err)
	}
	if next, err := s.Claim(ctx, testNow.Add(2*time.Minute), time.Second); err != nil || next != nil {
		t.Fatal("bypassed provider deadline", next, err)
	}
	s.View(ctx, func(tx *Tx) error {
		v, err := tx.Delivery(i.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if v.Intent.EventID != i.EventID || v.Intent.DestinationRevision != i.DestinationRevision || v.Intent.CreatedAt != i.CreatedAt || v.Intent.Attempts != 0 || len(v.Attempts) != 2 || v.Attempts[0].Outcome != "manual_retry" {
			t.Fatal(v)
		}
		return nil
	})
	if err := s.Update(ctx, func(tx *Tx) error { return tx.Retry(i.ID, testNow) }); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	next, err := s.Claim(ctx, testNow.Add(time.Hour), time.Second)
	if err != nil || next == nil || next.Attempts != 1 {
		t.Fatal(next, err)
	}
}
func TestEvidenceSequencesAndCascade(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	p, d := fixture(t)
	s.Update(ctx, func(tx *Tx) error {
		if err := transition(tx, p, d, "1", testNow); err != nil {
			return err
		}
		return tx.SaveReplay("event-1", map[string]string{"saved": "yes"})
	})
	if err := s.View(ctx, func(tx *Tx) error {
		page, err := tx.EvidenceInputs("event-1", 0)
		if err != nil || len(page.Observations) != 1 || page.Observations[0].Sequence != 1 {
			t.Fatal(page, err)
		}
		raw, err := tx.Observations("health", []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		var o watch.Observation
		json.Unmarshal(raw[0], &o)
		if o.Sequence != 1 {
			t.Fatal(o)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s.Update(ctx, func(tx *Tx) error {
		_, err := tx.sql.ExecContext(ctx, "DELETE FROM outbox;DELETE FROM events")
		return err
	})
	if count(t, s, "event_replays") != 0 {
		t.Fatal("orphaned checkpoint")
	}
}

func TestWatchInspectionPagesBoundLargeStateAndPayloads(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	p, d := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error {
		for n := 1; n <= 101; n++ {
			key := fmt.Sprintf("entity-%03d", n)
			if err := tx.SaveEntity("health", key, p.Revision, map[string]any{"open": true, "sourceUnhealthy": true, "matches": 2, "lastSequence": n, "samples": []int{1, 2}, "baseline": strings.Repeat("x", 10000)}, testNow); err != nil {
				return err
			}
			if err := transition(tx, p, d, key, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		first, err := tx.InspectSummary("health", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Entities) != 100 || len(first.Deliveries) != 100 || !first.EntitiesMore || !first.DeliveriesMore {
			t.Fatal(first)
		}
		raw, _ := json.Marshal(first)
		if bytes.Contains(raw, []byte("baseline")) || bytes.Contains(raw, []byte("payload")) {
			t.Fatal("bulk state in summary")
		}
		second, err := tx.InspectSummary("health", first.EntitiesAfter, first.DeliveriesBefore)
		if err != nil {
			t.Fatal(err)
		}
		// transition() also creates the common [] entity.
		if len(second.Entities) != 2 || len(second.Deliveries) != 1 || second.EntitiesMore || second.DeliveriesMore {
			t.Fatal(second)
		}
		unhealthy, open, err := tx.EntityHealth("health")
		if err != nil || unhealthy != 101 || open != 102 {
			t.Fatal(unhealthy, open, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
