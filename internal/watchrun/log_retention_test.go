package watchrun

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestLogRetentionPreservesPinnedAndUnrelatedHistory(t *testing.T) {
	a, _ := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest}); err != nil {
		t.Fatal(err)
	}
	push := strings.Replace(nativeLogManifest, "id: output", "id: pushed", 1)
	push = strings.Replace(push, "{type: logs, logs: {window: 1s, minEvents: 2}}", "{type: push}", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: push}); err != nil {
		t.Fatal(err)
	}
	log, _ := a.Record(ctx, "output")
	other, _ := a.Record(ctx, "pushed")
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		sequence := 0
		appendInput := func(r store.WatchRecord) (int64, error) {
			sequence++
			return tx.AppendObservation(r.Plan.Definition.Metadata.ID, r.Plan.Revision, r.Generation, watch.Observation{InputID: fmt.Sprint(sequence), Health: "waiting", AcceptedAt: start})
		}
		pinned, err := appendInput(log)
		if err != nil {
			return err
		}
		expired, err := appendInput(log)
		if err != nil {
			return err
		}
		unrelated, err := appendInput(other)
		if err != nil {
			return err
		}
		if err := tx.Retain(start.Add(-time.Hour), start.Add(2*time.Hour), []int64{pinned}, nil); err != nil {
			return err
		}
		if err := tx.RetainLogSummaries("output", start.Add(time.Hour)); err != nil {
			return err
		}
		if _, err := tx.Observations("output", []int64{pinned}); err != nil {
			return err
		}
		if _, err := tx.Observations("pushed", []int64{unrelated}); err != nil {
			return err
		}
		if _, err := tx.Observations("output", []int64{expired}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("unneeded summary not expired: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceLogSweepKeepsCurrentPinsAndUnrelatedHistory(t *testing.T) {
	a, _ := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest}); err != nil {
		t.Fatal(err)
	}
	push := strings.Replace(nativeLogManifest, "id: output", "id: pushed", 1)
	push = strings.Replace(push, "{type: logs, logs: {window: 1s, minEvents: 2}}", "{type: push}", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: push}); err != nil {
		t.Fatal(err)
	}
	log, _ := a.Record(ctx, "output")
	other, _ := a.Record(ctx, "pushed")
	var pinned, unrelated int64
	if err := a.Store.Update(ctx, func(tx *store.Tx) error {
		var err error
		pinned, err = tx.AppendObservation("output", log.Plan.Revision, log.Generation, watch.Observation{InputID: "pinned", Health: "waiting", AcceptedAt: start})
		if err != nil {
			return err
		}
		for i := range 150 {
			if _, err = tx.AppendObservation("output", log.Plan.Revision, log.Generation, watch.Observation{InputID: fmt.Sprint(i), Health: "waiting", AcceptedAt: start}); err != nil {
				return err
			}
		}
		unrelated, err = tx.AppendObservation("pushed", other.Plan.Revision, other.Generation, watch.Observation{InputID: "unrelated", Health: "waiting", AcceptedAt: start})
		if err != nil {
			return err
		}
		return tx.SaveEntity("output", "[]", log.Plan.Revision, condition.State{LastSequence: pinned}, start)
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Maintain(ctx, start.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		u, err := tx.Usage()
		if err != nil {
			return err
		}
		if u.Observations != 2 {
			t.Error("log sweep removed unrelated history or kept expired rows", u.Observations)
		}
		if _, err = tx.Observations("output", []int64{pinned}); err != nil {
			return err
		}
		_, err = tx.Observations("pushed", []int64{unrelated})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
