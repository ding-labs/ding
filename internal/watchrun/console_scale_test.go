package watchrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

// Opt-in release qualification with real SQLite and concurrent accepted inputs / delivery.
func TestConsoleScaleQualification(t *testing.T) {
	if os.Getenv("DING_CONSOLE_SCALE") != "1" {
		t.Skip("set DING_CONSOLE_SCALE=1 for the 1000-watch/100000-event qualification")
	}
	a, _ := setup(t)
	a.Now = func() time.Time { return time.Now().UTC() }
	a.Output = io.Discard
	b, err := plan.Parse([]byte(`apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: console}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: scale}
spec:
  source: {type: push}
  condition: {field: status, operator: gte, value: 500}
  policy: {trigger: level, interval: 0s}
  destinations: [{ref: console, events: [firing]}]
`))
	if err != nil {
		t.Fatal(err)
	}
	records := make([]store.WatchRecord, 1000)
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		if err := tx.SaveDestination(b.Destinations[0], a.Now()); err != nil {
			return err
		}
		for i := range records {
			d := b.Watches[0].Definition
			d.Metadata.ID = fmt.Sprintf("scale-%04d", i)
			p, err := plan.Compile(d)
			if err != nil {
				return err
			}
			records[i] = store.WatchRecord{Plan: p, Generation: 1, Status: "running", NextAt: a.Now()}
			if err = tx.SaveWatch(records[i], a.Now()); err != nil {
				return err
			}
			for entity := 0; entity < 20; entity++ {
				key := fmt.Sprintf("entity-%02d", entity)
				if err = tx.SaveEntity(d.Metadata.ID, key, p.Revision, map[string]any{"entity": key, "open": entity%5 == 0, "matches": entity % 3, "sourceUnhealthy": entity%9 == 0}, a.Now()); err != nil {
					return err
				}
			}
		}
		for i := 0; i < 100000; i++ {
			r := records[i%1000]
			if _, err := tx.AppendEvent(watch.Event{ID: fmt.Sprintf("history-%06d", i), WatchID: r.Plan.Definition.Metadata.ID, Revision: r.Plan.Revision, At: a.Now(), Type: "firing", Message: "Scale fixture"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var accepted atomic.Int64
	worker := make(chan error, 1)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-runCtx.Done():
				worker <- nil
				return
			default:
			}
			_, err := a.Accept(runCtx, records[0], source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"status": 503}}}}, fmt.Sprint("live-", i), a.Now())
			if err != nil {
				if runCtx.Err() != nil {
					worker <- nil
				} else {
					worker <- err
				}
				return
			}
			accepted.Add(1)
			if _, err = a.DeliverOne(runCtx); err != nil && runCtx.Err() == nil {
				worker <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	latencies := []time.Duration{}
	for i := 0; i < 30; i++ {
		started := time.Now()
		w, err := a.ConsoleWatches(ctx, store.ConsoleQuery{Limit: 50})
		if err != nil || len(w.Watches) != 50 || w.All != 1000 {
			t.Fatal(w.All, err)
		}
		if err = a.Store.View(ctx, func(tx *store.Tx) error {
			p, err := tx.ConsoleEvents(store.ConsoleQuery{Limit: 50})
			if err == nil && (len(p.Events) != 50 || p.Total < 100000) {
				return fmt.Errorf("wrong event total: %d", p.Total)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		latencies = append(latencies, time.Since(started))
	}
	cancel()
	if err = <-worker; err != nil {
		t.Fatal(err)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[28]
	t.Logf("1000 watches / 100000 events: paired list p50=%s p95=%s; %d live accepts with delivery during polling", latencies[15], p95, accepted.Load())
	if p95 > time.Second {
		t.Fatalf("warm list budget exceeded: %s", p95)
	}
	if accepted.Load() < 10 {
		t.Fatal("UI polling starved acquisition", accepted.Load())
	}
}
