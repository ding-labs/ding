package watchrun

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/store"
)

func TestFailedReceiptRollsBackLogCheckpointAndIncident(t *testing.T) {
	a, dir := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest}); err != nil {
		t.Fatal(err)
	}
	session, err := a.StartLogSession(ctx, "output")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := a.Record(ctx, "output")
	// This second connection only installs a failure trigger in the private test
	// database. The failure occurs at the last write in the acceptance transaction.
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_log_receipt BEFORE INSERT ON input_receipts WHEN NEW.input_id LIKE 'log:%' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	now := start.Add(time.Second)
	a.Now = func() time.Time { return now }
	input := LogSnapshot{Session: session.ID, Sequence: 1, Summary: logs.Summary{Version: 1, WindowSeconds: 1, WindowStart: start, WindowEnd: now, Total: 2, Classified: 2, Warnings: 2, Complete: true}}
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); err == nil {
		t.Fatal("injected commit failure was ignored")
	}
	after, _ := a.Record(ctx, "output")
	if after.Cursor != before.Cursor {
		t.Fatal("checkpoint advanced without the receipt")
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		events, err := tx.Events("output", 0, 100)
		for _, event := range events {
			if event.Type == "firing" {
				t.Error("incident survived rollback")
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TRIGGER fail_log_receipt"); err != nil {
		t.Fatal(err)
	}
	receipt, err := a.AcceptLogSnapshot(ctx, "output", input)
	if err != nil || receipt.Duplicate {
		t.Fatal("retry was not a new atomic commit", receipt, err)
	}
	duplicate, err := a.AcceptLogSnapshot(ctx, "output", input)
	if err != nil || !duplicate.Duplicate || duplicate.First != receipt.First {
		t.Fatal(duplicate, err)
	}
}
