package watchrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestRevisionPreserveResetAndFencing(t *testing.T) {
	a, _ := setup(t)
	old := apply(t, a, "https://example.com")
	input(t, a, old, 1, 500)
	input(t, a, old, 2, 500)
	changed := strings.Replace(manifest("https://example.com"), "metadata: {id: api}", "metadata: {id: api, name: Renamed}", 1)
	preview, err := a.Apply(ctx, ApplyRequest{Manifest: changed, DryRun: true})
	if err != nil || preview.Changes[0].State != "preserved" {
		t.Fatal(preview, err)
	}
	same, _ := a.Inspect(ctx, "api")
	if same.Watch.Generation != old.Generation {
		t.Fatal("dry run changed generation")
	}
	result, err := a.Apply(ctx, ApplyRequest{Manifest: changed, Expected: map[string]string{"api": old.Plan.Revision}})
	if err != nil || result.Changes[0].State != "preserved" {
		t.Fatal(result, err)
	}
	current, _ := a.Inspect(ctx, "api")
	if current.Watch.Generation != old.Generation+1 || state(t, a).Matches != 2 {
		t.Fatal("compatible state lost")
	}
	if _, err := a.Accept(ctx, old, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 500}}}}, "stale", start.Add(15*time.Second)); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	input(t, a, current.Watch, 3, 500)
	if !state(t, a).Open {
		t.Fatal("preserved streak did not fire")
	}
	changed = strings.Replace(changed, "value: 500", "value: 600", 1)
	result, err = a.Apply(ctx, ApplyRequest{Manifest: changed})
	if err != nil || result.Changes[0].State != "reset" {
		t.Fatal(result, err)
	}
	current, _ = a.Inspect(ctx, "api")
	if len(current.Entities) != 0 || len(current.Deliveries) != 1 {
		t.Fatal("reset must preserve queued event", current)
	}
	var events []watch.Event
	if err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; events, err = tx.Events("api", 0, 100); return err }); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Type == "state_reset" {
			found = true
		}
	}
	if !found {
		t.Fatal("silent reset")
	}
}
func TestPauseResumeDeleteAndCancellation(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	now := start.Add(20 * time.Second)
	a.Now = func() time.Time { return now }
	paused, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"})
	if err != nil || paused.Status != "paused" || !state(t, a).Open {
		t.Fatal(paused, err)
	}
	if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "unknown"}}}, "paused", now); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	resumed, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if !state(t, a).Open || state(t, a).Matches != 0 {
		t.Fatal("resume continuity incorrect")
	}
	batch := source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}
	if _, err := a.Accept(ctx, resumed, batch, "recover1", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Accept(ctx, resumed, batch, "recover2", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	deleted, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "delete"})
	if err != nil || deleted.Status != "deleted" {
		t.Fatal(deleted, err)
	}
	inspection, _ := a.Inspect(ctx, "api")
	for _, job := range inspection.Deliveries {
		if job.Status != "pending" {
			t.Fatal("delete canceled without request")
		}
	}
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "delete", CancelPending: true}); err != nil {
		t.Fatal(err)
	}
	inspection, _ = a.Inspect(ctx, "api")
	for _, job := range inspection.Deliveries {
		if job.Status != "canceled" {
			t.Fatal(job)
		}
	}
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("resurrected tombstone")
	}
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest("https://example.com")}); err == nil {
		t.Fatal("reapplied tombstone")
	}
}
func missingManifest() string {
	return strings.Replace(strings.Replace(manifest("https://example.com"), "field: http.status, operator: gte, value: 500", "missingFor: 10s", 1), "consecutive: 3, recoverAfter: 2", "consecutive: 1, recoverAfter: 1", 1)
}
func TestMissingTimersPersistFenceAndDoNotPoll(t *testing.T) {
	a, dir := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: missingManifest()}); err != nil {
		t.Fatal(err)
	}
	before, _ := a.Inspect(ctx, "api")
	if err := a.RunTimers(ctx, start.Add(9*time.Second)); err != nil {
		t.Fatal(err)
	}
	if state(t, a).Open {
		t.Fatal("early firing")
	}
	a.Store.Close()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	if err := a.RunTimers(ctx, start.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !state(t, a).Open {
		t.Fatal("missed persisted deadline")
	}
	after, _ := a.Inspect(ctx, "api")
	if !after.Watch.NextAt.Equal(before.Watch.NextAt) || !after.Watch.LastInputAt.IsZero() {
		t.Fatal("timer advanced poll checkpoint")
	}
	if err := a.RunTimers(ctx, start.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	after, _ = a.Inspect(ctx, "api")
	if len(after.Deliveries) != 1 {
		t.Fatal("repeated timer firing")
	}
	batch := source.Batch{Observations: []watch.Observation{{Health: "unchanged"}}}
	if _, err := a.Accept(ctx, after.Watch, batch, "fresh", start.Add(12*time.Second)); err != nil {
		t.Fatal(err)
	}
	if state(t, a).Open {
		t.Fatal("freshness did not recover absence")
	}
	stale := source.Batch{Entity: "[]", Deadline: start.Add(10 * time.Second), Observations: []watch.Observation{{Health: "timer"}}}
	if _, err := a.Accept(ctx, after.Watch, stale, "stale-timer", start.Add(30*time.Second)); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	a.Now = func() time.Time { return start.Add(15 * time.Second) }
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RunTimers(ctx, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if state(t, a).Open {
		t.Fatal("timer fired while paused")
	}
	a.Now = func() time.Time { return start.Add(time.Hour) }
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RunTimers(ctx, start.Add(time.Hour+9*time.Second)); err != nil {
		t.Fatal(err)
	}
	if state(t, a).Open {
		t.Fatal("resume fired historical silence")
	}
	if err := a.RunTimers(ctx, start.Add(time.Hour+10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !state(t, a).Open {
		t.Fatal("resumed timer missing")
	}
}
func TestQuotasRollbackAndRecoveredGap(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	a.Limits.MaxPending = 1
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	before, _ := a.Inspect(ctx, "api")
	_, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}, "blocked", start.Add(20*time.Second))
	if !errors.Is(err, ErrQuota) {
		t.Fatal(err)
	}
	after, _ := a.Inspect(ctx, "api")
	if !after.Watch.NextAt.Equal(before.Watch.NextAt) || after.Watch.LastError != "quota_exceeded" || state(t, a).Recoveries != 0 {
		t.Fatal("quota advanced state", after)
	}
	if err := a.Store.Update(ctx, func(tx *store.Tx) error { return tx.CancelDeliveries("api", start.Add(21*time.Second)) }); err != nil {
		t.Fatal(err)
	}
	input(t, a, r, 5, 200)
	var events []watch.Event
	a.Store.View(ctx, func(tx *store.Tx) error { events, _ = tx.Events("api", 0, 100); return nil })
	found := false
	for _, event := range events {
		if event.Type == "gap" {
			found = true
		}
	}
	if !found {
		t.Fatal("backpressure gap hidden")
	}
	a.Limits.MaxWatches = 1
	more := strings.Replace(manifest("https://example.com"), "id: api}", "id: second}", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: more}); !errors.Is(err, ErrQuota) {
		t.Fatal("watch quota ignored", err)
	}
	a.Limits.MaxBytes = 1
	if _, err := a.Budget(ctx); !errors.Is(err, ErrQuota) {
		t.Fatal(err)
	}
}
func TestGroupedBootstrapDoesNotConsumeEntityQuota(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "  condition:", "  groupBy: [host]\n  limits: {maxEntities: 1}\n  condition:", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	i, _ := a.Inspect(ctx, "api")
	r := i.Watch
	if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "unknown"}}}, "unavailable", start); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"host": "a", "http.status": 200}}}}, "known", start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	i, _ = a.Inspect(ctx, "api")
	if len(i.Entities) != 1 || i.Entities[condition.SourceEntity] != nil {
		t.Fatal("bootstrap health leaked entity", i.Entities)
	}
	if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"host": "b", "http.status": 200}}}}, "extra", start.Add(2*time.Second)); !errors.Is(err, ErrQuota) {
		t.Fatal(err)
	}
}
func TestRetentionPinsOpenIncidentAndPendingEvidence(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	a.Limits.Retention = time.Hour
	now := start.Add(48 * time.Hour)
	if err := a.Maintain(ctx, now); err != nil {
		t.Fatal(err)
	}
	var usage store.Usage
	var events []watch.Event
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		usage, err = tx.Usage()
		if err != nil {
			return err
		}
		events, err = tx.Events("api", 0, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if usage.Pending != 1 || usage.Entities != 1 || usage.Observations < 3 || len(events) != 1 || events[0].Type != "firing" {
		t.Fatal(usage, events)
	}
	if err := a.Store.Update(ctx, func(tx *store.Tx) error { return tx.CancelDeliveries("api", now) }); err != nil {
		t.Fatal(err)
	}
	if err := a.Maintain(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; events, err = tx.Events("api", 0, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatal("active incident lost event")
	}
	for n := 0; n < 2; n++ {
		if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}, fmt.Sprintf("recover-%d", n), now.Add(time.Duration(n)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	a.Store.Update(ctx, func(tx *store.Tx) error { return tx.CancelDeliveries("api", now) })
	if err := a.Maintain(ctx, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	i, _ := a.Inspect(ctx, "api")
	if len(i.Entities) != 0 || len(i.Deliveries) != 0 {
		t.Fatal("idle/terminal history not expired", i)
	}
	a.Store.View(ctx, func(tx *store.Tx) error { usage, _ = tx.Usage(); return nil })
	if usage.Observations != 0 {
		t.Fatal("unneeded evidence retained", usage)
	}
}
func TestConcurrentLifecycleCancelsOldSource(t *testing.T) {
	a, _ := setup(t)
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer endpoint.Close()
	a.Now = func() time.Time { return time.Now().UTC() }
	apply(t, a, endpoint.URL)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Run(runCtx) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("source not started")
	}
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("pause did not cancel source")
	}
	var wg sync.WaitGroup
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"})
			_, _ = a.Inspect(ctx, "api")
		}()
	}
	wg.Wait()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest(endpoint.URL)}); !errors.Is(err, ErrClosing) {
		t.Fatal("admitted after shutdown", err)
	}
	i, _ := a.Inspect(ctx, "api")
	if len(i.Entities) != 0 {
		var s condition.State
		for _, data := range i.Entities {
			json.Unmarshal(data, &s)
		}
		t.Fatal("canceled response became observation", s)
	}
}

func TestDedupHorizonAndWindowRetention(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "field: http.status, operator: gte, value: 500", "field: http.status, numeric: 'avg(value) over 24h > 1000'", 1)
	m = strings.Replace(m, "  condition:", "  limits: {idleTTL: 1h}\n  condition:", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	i, _ := a.Inspect(ctx, "api")
	r := i.Watch
	batch := source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}
	first, err := a.Accept(ctx, r, batch, "id", start)
	if err != nil {
		t.Fatal(err)
	}
	a.Limits.Retention = time.Hour
	if err := a.Maintain(ctx, start.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	i, _ = a.Inspect(ctx, "api")
	if len(i.Entities) != 1 || len(state(t, a).Samples) != 1 {
		t.Fatal("evicted active window")
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error { _, err := tx.Observations("api", []int64{first.First}); return err }); err != nil {
		t.Fatal("lost active sample evidence", err)
	}
	again, err := a.Accept(ctx, r, batch, "id", start.Add(25*time.Hour))
	if err != nil || again.Duplicate || again.First == first.First {
		t.Fatal("expired idempotency identity cannot be reused", again, err)
	}
}
