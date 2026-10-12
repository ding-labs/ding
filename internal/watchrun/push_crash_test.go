package watchrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

func TestPushProcessLossBeforeCommitAndBeforeResponse(t *testing.T) {
	if mode := os.Getenv("DING_PUSH_CRASH_MODE"); mode != "" {
		dir := os.Getenv("DING_PUSH_CRASH_DIR")
		db, err := store.Open(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		app := New(db)
		record, err := app.Record(context.Background(), "push")
		if err != nil {
			t.Fatal(err)
		}
		raw := []byte(`{"value":9}`)
		input := NewPushInput("producer", "stable", raw, 100, time.Hour)
		input.ValidUntil = time.Now().Add(time.Hour)
		batch, err := PreparePush(context.Background(), record, raw, 32, 32000)
		if err != nil {
			t.Fatal(err)
		}
		checkpoint := func() {
			if err := os.WriteFile(filepath.Join(dir, "barrier"), []byte(mode), 0600); err != nil {
				t.Fatal(err)
			}
			select {}
		}
		calls := 0
		app.Now = func() time.Time {
			calls++
			if mode == "before-commit" && calls == 2 {
				checkpoint()
			}
			return time.Now().UTC()
		}
		receipt, err := app.CommitPush(context.Background(), record, batch, input)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "acceptance"), []byte(receipt.AcceptanceID), 0600); err != nil {
			t.Fatal(err)
		}
		checkpoint()
		return
	}
	for _, mode := range []string{"before-commit", "before-response"} {
		t.Run(mode, func(t *testing.T) {
			app, dir := setup(t)
			if _, err := app.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
				t.Fatal(err)
			}
			app.Store.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestPushProcessLossBeforeCommitAndBeforeResponse$")
			cmd.Env = append(os.Environ(), "DING_PUSH_CRASH_MODE="+mode, "DING_PUSH_CRASH_DIR="+dir)
			var logs bytes.Buffer
			cmd.Stdout = &logs
			cmd.Stderr = &logs
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(dir, "barrier")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					cmd.Wait()
					t.Fatal("barrier not reached", logs.String())
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			db, err := store.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			app = New(db)
			record, _ := app.Record(ctx, "push")
			raw := []byte(`{"value":9}`)
			input := NewPushInput("producer", "stable", raw, 100, time.Hour)
			batch, err := PreparePush(ctx, record, raw, 32, 32000)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := app.CommitPush(ctx, record, batch, input)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Duplicate != (mode == "before-response") {
				t.Fatal("wrong recovery receipt", receipt)
			}
			if mode == "before-response" {
				prior, _ := os.ReadFile(filepath.Join(dir, "acceptance"))
				if receipt.AcceptanceID != string(prior) {
					t.Fatal("lost committed acceptance")
				}
			}
			if err := db.View(ctx, func(tx *store.Tx) error {
				u, e := tx.PushUsage()
				if u.Requests != 1 || u.Observations != 1 {
					return fmt.Errorf("reconciliation: %+v", u)
				}
				return e
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
