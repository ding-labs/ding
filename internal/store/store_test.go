package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

var testNow = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T) (plan.Compiled, plan.CompiledDestination) {
	t.Helper()
	c, err := plan.Compile(watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "health"}, Spec: watch.Spec{Source: watch.Source{Type: "http", URL: "https://example.com"}, Condition: watch.Condition{Field: "http.status", Operator: "gte", Value: 500}, Destinations: []watch.Target{{Ref: "ops"}}}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := plan.CompileDestination(watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: "ops"}, Spec: watch.DestinationSpec{Type: "console"}})
	if err != nil {
		t.Fatal(err)
	}
	return c, d
}
func openTest(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func seed(t *testing.T, s *Store) {
	t.Helper()
	c, d := fixture(t)
	if err := s.Update(context.Background(), func(tx *Tx) error {
		if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
			return err
		}
		return tx.SaveDestination(d, testNow)
	}); err != nil {
		t.Fatal(err)
	}
}
func transition(tx *Tx, c plan.Compiled, d plan.CompiledDestination, input string, now time.Time) error {
	seq, err := tx.AppendObservation(c.Definition.Metadata.ID, c.Revision, 1, watch.Observation{InputID: input, AcceptedAt: now, Health: "ok", Fields: map[string]any{"http.status": 500}})
	if err != nil {
		return err
	}
	if err = tx.SaveEntity(c.Definition.Metadata.ID, "[]", c.Revision, map[string]any{"open": true, "lastSequence": seq}, now); err != nil {
		return err
	}
	event := watch.Event{ID: "event-" + input, WatchID: c.Definition.Metadata.ID, Revision: c.Revision, Type: "firing", At: now, Evidence: []int64{seq}}
	if _, err = tx.AppendEvent(event); err != nil {
		return err
	}
	payload, _ := json.Marshal(event)
	if _, err = tx.Enqueue(Intent{EventID: event.ID, WatchID: event.WatchID, DestinationID: d.Definition.Metadata.ID, DestinationRevision: d.Revision, Payload: payload, NextAt: now, CreatedAt: now}); err != nil {
		return err
	}
	if err = tx.SaveReceipt(event.WatchID, 1, input, seq, seq, now.Add(24*time.Hour)); err != nil {
		return err
	}
	if err = tx.SaveTimer(Timer{WatchID: event.WatchID, Entity: "[]", Kind: "missing", Generation: 1, Due: now.Add(time.Minute)}); err != nil {
		return err
	}
	return tx.Checkpoint(event.WatchID, 1, now.Add(5*time.Second), now, input)
}
func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestDurabilitySettingsAndWriterOwnership(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	h, err := s.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.SQLite != "3.53.4" || h.Schema != SchemaVersion || h.Journal != "wal" || h.Synchronous != 2 || h.Integrity != "ok" {
		t.Fatal(h)
	}
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("second writer admitted")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err == nil {
		if _, err = Open(context.Background(), alias); err == nil {
			t.Fatal("symlink writer admitted")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(context.Background(), func(*Tx) error { return nil }); err != ErrClosed {
		t.Fatal(err)
	}
	next := openTest(t, dir)
	if _, err := next.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestAtomicTransitionAndRollback(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	sentinel := errors.New("injected failure before commit")
	err := s.Update(context.Background(), func(tx *Tx) error {
		if err := transition(tx, c, d, "1", testNow); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, table := range []string{"observations", "entities", "events", "outbox", "timers", "input_receipts"} {
		if n := count(t, s, table); n != 0 {
			t.Fatalf("%s retained %d rolled-back rows", table, n)
		}
	}
	if err = s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "1", testNow) }); err != nil {
		t.Fatal(err)
	}
	if err = s.View(context.Background(), func(tx *Tx) error {
		r, err := tx.Watch("health")
		if err != nil {
			return err
		}
		if r.Cursor != "1" || !r.LastInputAt.Equal(testNow) {
			t.Fatal(r)
		}
		state, err := tx.Entity("health", "[]")
		if err != nil || !strings.Contains(string(state), `"open":true`) {
			t.Fatal(string(state), err)
		}
		events, err := tx.Events("health", 0, 100)
		if err != nil || len(events) != 1 || events[0].Sequence != 1 {
			t.Fatal(events, err)
		}
		first, last, err := tx.Receipt("health", 1, "1", testNow)
		if err != nil || first != 1 || last != 1 {
			t.Fatal(first, last, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "1", testNow) }); err == nil {
		t.Fatal("duplicate input accepted")
	}
	if count(t, s, "events") != 1 {
		t.Fatal("duplicate logical event")
	}
}
func TestImmutableRevisionsAndForeignKeys(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	for _, statement := range []string{`UPDATE watch_revisions SET definition='{}'`, `UPDATE destination_revisions SET definition='{}'`, `INSERT INTO watches(id,revision,generation,status,next_at) VALUES('missing','missing',1,'running',0)`} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Fatal("accepted", statement)
		}
	}
	err := s.Update(context.Background(), func(tx *Tx) error { return tx.Checkpoint("health", 99, testNow, testNow, "bad") })
	if !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}
func TestBackupAndFailedMigration(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	seed(t, s)
	c, d := fixture(t)
	if err := s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "1", testNow) }); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(context.Background(), backup); err == nil {
		t.Fatal("overwrote backup")
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	restoreDir := t.TempDir()
	if err = os.WriteFile(filepath.Join(restoreDir, "ding.db"), data, 0600); err != nil {
		t.Fatal(err)
	}
	restored := openTest(t, restoreDir)
	if count(t, restored, "events") != 1 || count(t, restored, "outbox") != 1 {
		t.Fatal("incomplete backup")
	}
	err = s.migrate(context.Background(), 4, []string{initialSchema, inspectionSchema, queueIndexSchema, "CREATE TABLE failed_migration(id INTEGER); THIS IS NOT SQL;"})
	if err == nil {
		t.Fatal("migration should fail")
	}
	var version int
	s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != SchemaVersion {
		t.Fatal("schema version advanced")
	}
	if _, err = s.db.Exec("SELECT * FROM failed_migration"); err == nil {
		t.Fatal("failed migration table survived")
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "before-schema-4-*.db"))
	if len(backups) != 1 {
		t.Fatal("migration lacked backup")
	}
	s.Close()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("PRAGMA user_version=99")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), dir); err == nil {
		t.Fatal("opened newer schema")
	}
}
func TestOutboxLeaseRecoveryOrderAndFencing(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	for _, id := range []string{"1", "2"} {
		if err := s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, id, testNow) }); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.Claim(context.Background(), testNow, time.Second)
	if err != nil || first == nil || first.EventID != "event-1" {
		t.Fatal(first, err)
	}
	if blocked, err := s.Claim(context.Background(), testNow, time.Second); err != nil || blocked != nil {
		t.Fatal("recovery overtook firing", blocked, err)
	}
	reclaimed, err := s.Claim(context.Background(), testNow.Add(time.Second), time.Second)
	if err != nil || reclaimed == nil || reclaimed.ID != first.ID || reclaimed.LeaseToken == first.LeaseToken {
		t.Fatal(reclaimed, err)
	}
	if err := s.Finish(context.Background(), *first, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); err != ErrStale {
		t.Fatal("stale ack accepted", err)
	}
	if err = s.Finish(context.Background(), *reclaimed, delivery.Result{Outcome: delivery.Retryable}, testNow.Add(time.Minute), testNow); err != nil {
		t.Fatal(err)
	}
	if next, err := s.Claim(context.Background(), testNow.Add(10*time.Second), time.Second); err != nil || next != nil {
		t.Fatal("retry deadline ignored", next, err)
	}
	retry, err := s.Claim(context.Background(), testNow.Add(time.Minute), time.Second)
	if err != nil || retry == nil {
		t.Fatal(retry, err)
	}
	if err = s.Finish(context.Background(), *retry, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	next, err := s.Claim(context.Background(), testNow.Add(time.Minute), time.Second)
	if err != nil || next == nil || next.EventID != "event-2" {
		t.Fatal(next, err)
	}
	if count(t, s, "delivery_attempts") != 2 {
		t.Fatal("attempt history missing")
	}
}
func TestConcurrentTransactions(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			errs <- s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, fmt.Sprint(i), testNow) })
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count(t, s, "events") != 20 {
		t.Fatal("lost transaction")
	}
}
func TestDiskFullAndReadonlyRollback(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	var pages int
	s.db.QueryRow("PRAGMA page_count").Scan(&pages)
	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
		t.Fatal(err)
	}
	err := s.Update(context.Background(), func(tx *Tx) error {
		if err := transition(tx, c, d, "full", testNow); err != nil {
			return err
		}
		return tx.SaveEntity("health", "large", c.Revision, strings.Repeat("x", 1<<20), testNow)
	})
	if err == nil {
		t.Fatal("disk page budget did not fail")
	}
	if count(t, s, "events") != 0 || count(t, s, "observations") != 0 {
		t.Fatal("partial disk-full transaction")
	}
	s.db.Exec("PRAGMA max_page_count=262144;PRAGMA query_only=ON")
	if err = s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "readonly", testNow) }); err == nil {
		t.Fatal("readonly write succeeded")
	}
	s.db.Exec("PRAGMA query_only=OFF")
	if count(t, s, "events") != 0 {
		t.Fatal("readonly state changed")
	}
}
func TestCrashTransactionBoundaries(t *testing.T) {
	if mode := os.Getenv("DING_STORE_CRASH"); mode != "" {
		s, err := Open(context.Background(), os.Getenv("DING_STORE_DIR"))
		if err != nil {
			t.Fatal(err)
		}
		c, d := fixture(t)
		if mode == "before" {
			_ = s.Update(context.Background(), func(tx *Tx) error {
				if err := transition(tx, c, d, "crash", testNow); err != nil {
					t.Fatal(err)
				}
				os.Exit(73)
				return nil
			})
		}
		if err = s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "crash", testNow) }); err != nil {
			t.Fatal(err)
		}
		if mode == "leased" {
			if _, err = s.Claim(context.Background(), testNow, time.Second); err != nil {
				t.Fatal(err)
			}
		}
		os.Exit(73)
	}
	for _, mode := range []string{"before", "after", "leased"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			s := openTest(t, dir)
			seed(t, s)
			s.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashTransactionBoundaries$")
			cmd.Env = append(os.Environ(), "DING_STORE_CRASH="+mode, "DING_STORE_DIR="+dir)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 73 {
				t.Fatalf("helper: %s %v", output, err)
			}
			reopened := openTest(t, dir)
			want := 1
			if mode == "before" {
				want = 0
			}
			for _, table := range []string{"observations", "entities", "events", "outbox", "timers", "input_receipts"} {
				if n := count(t, reopened, table); n != want {
					t.Fatalf("%s got %d want %d", table, n, want)
				}
			}
			if mode == "leased" {
				i, err := reopened.Claim(context.Background(), testNow.Add(2*time.Second), time.Second)
				if err != nil || i == nil || i.EventID != "event-crash" {
					t.Fatal("lease not recovered", i, err)
				}
			}
		})
	}
}

func TestWriterLockAcrossProcess(t *testing.T) {
	if os.Getenv("DING_LOCK_HELPER") == "1" {
		s, err := Open(context.Background(), os.Getenv("DING_STORE_DIR"))
		if err == nil {
			s.Close()
			os.Exit(74)
		}
		if !strings.Contains(err.Error(), "already has a writer") {
			os.Exit(75)
		}
		os.Exit(73)
	}
	dir := t.TempDir()
	s := openTest(t, dir)
	cmd := exec.Command(os.Args[0], "-test.run=^TestWriterLockAcrossProcess$")
	cmd.Env = append(os.Environ(), "DING_LOCK_HELPER=1", "DING_STORE_DIR="+dir)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("second-process lock: %s %v", output, err)
	}
	s.Close()
	if _, err := os.Stat(filepath.Join(dir, "writer.lock")); err != nil {
		t.Fatal("lock file was removed; inode race possible", err)
	}
}
func TestDestinationRevisionPinnedAndIndependentDispatch(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	if err := s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "old-destination", testNow) }); err != nil {
		t.Fatal(err)
	}
	revised := d.Definition
	revised.Spec.Type = "webhook"
	revised.Spec.URLRef = &watch.SecretRef{Env: "ROTATING_SECRET"}
	changed, err := plan.CompileDestination(revised)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(context.Background(), func(tx *Tx) error { return tx.SaveDestination(changed, testNow) }); err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(context.Background(), testNow, time.Minute)
	if err != nil || job == nil || job.Destination.Definition.Spec.Type != "console" {
		t.Fatal("rewrote queued destination", job, err)
	}
	other := c.Definition
	other.Metadata.ID = "other"
	otherPlan, err := plan.Compile(other)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(context.Background(), func(tx *Tx) error {
		if err := tx.SaveWatch(WatchRecord{Plan: otherPlan, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
			return err
		}
		return transition(tx, otherPlan, changed, "independent", testNow)
	}); err != nil {
		t.Fatal(err)
	}
	independent, err := s.Claim(context.Background(), testNow, time.Second)
	if err != nil || independent == nil || independent.WatchID != "other" {
		t.Fatal("one watch blocks other", independent, err)
	}
}
func TestStoreReadAndErrorContracts(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	if err := s.View(context.Background(), func(tx *Tx) error {
		watches, err := tx.Watches()
		if err != nil || len(watches) != 1 {
			t.Fatal(watches, err)
		}
		if _, err := tx.Watch("missing"); err != ErrNotFound {
			t.Fatal(err)
		}
		if _, err := tx.Entity("health", "missing"); err != ErrNotFound {
			t.Fatal(err)
		}
		if _, err := tx.Destination("missing", ""); err != ErrNotFound {
			t.Fatal(err)
		}
		if _, err := tx.Destination("ops", ""); err != nil {
			t.Fatal(err)
		}
		if _, _, err := tx.Receipt("health", 1, "missing", testNow); err != ErrNotFound {
			t.Fatal(err)
		}
		if _, err := tx.Events("", 0, 1001); err == nil {
			t.Fatal("unbounded event query")
		}
		if _, err := tx.Intent(999); err != ErrNotFound {
			t.Fatal(err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Update(ctx, func(*Tx) error { return nil }); err == nil {
		t.Fatal("canceled transaction started")
	}
	if _, err := s.Claim(context.Background(), testNow, 0); err == nil {
		t.Fatal("zero lease")
	}
	if err := s.Finish(context.Background(), Intent{}, delivery.Result{Outcome: "invented"}, testNow, testNow); err == nil {
		t.Fatal("invalid outcome")
	}
	s.Close()
	if _, err := s.Health(context.Background()); err != ErrClosed {
		t.Fatal(err)
	}
	if err := s.View(context.Background(), func(*Tx) error { return nil }); err != ErrClosed {
		t.Fatal(err)
	}
	if err := s.Backup(context.Background(), filepath.Join(t.TempDir(), "closed.db")); err != ErrClosed {
		t.Fatal(err)
	}
}
