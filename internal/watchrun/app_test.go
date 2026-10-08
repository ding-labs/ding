package watchrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

var ctx = context.Background()
var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func manifest(endpoint string) string {
	return fmt.Sprintf(`apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: hook}
spec: {type: webhook, urlRef: {env: WEBHOOK_URL}}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api}
spec:
  source: {type: http, url: %q, every: 5s, timeout: 2s}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {consecutive: 3, recoverAfter: 2}
  destinations: [{ref: hook, events: [firing, recovered]}]
`, endpoint)
}
func setup(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a := New(s)
	a.Now = func() time.Time { return start }
	return a, dir
}
func apply(t *testing.T, a *App, endpoint string) store.WatchRecord {
	t.Helper()
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest(endpoint)}); err != nil {
		t.Fatal(err)
	}
	i, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	return i.Watch
}
func input(t *testing.T, a *App, r store.WatchRecord, n int, status int) Receipt {
	t.Helper()
	receipt, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": status}}}}, fmt.Sprint(n), start.Add(time.Duration(n)*5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
func state(t *testing.T, a *App) condition.State {
	t.Helper()
	i, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	var s condition.State
	if err := json.Unmarshal(i.Entities["[]"], &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAtomicAcceptanceDedupAndRestart(t *testing.T) {
	a, dir := setup(t)
	record := apply(t, a, "https://example.com")
	input(t, a, record, 1, 500)
	input(t, a, record, 2, 502)
	receipt := input(t, a, record, 3, 503)
	if !state(t, a).Open {
		t.Fatal("did not open")
	}
	duplicate := input(t, a, record, 3, 503)
	if !duplicate.Duplicate || duplicate.First != receipt.First {
		t.Fatal(duplicate)
	}
	inspection, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Deliveries) != 1 || inspection.Deliveries[0].Status != "pending" {
		t.Fatal(inspection)
	}
	before := inspection.Watch
	// The first output would recover partially, but the second has an invalid entity.
	bad := source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}, {Health: "ok", Fields: map[string]any{"http.status": make(chan int)}}}}
	if _, err := a.Accept(ctx, record, bad, "bad", start.Add(20*time.Second)); err == nil {
		t.Fatal("accepted unserializable batch")
	}
	after, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Watch.NextAt.Equal(before.NextAt) || state(t, a).Recoveries != 0 {
		t.Fatal("partial checkpoint/state commit")
	}
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	if !state(t, a).Open {
		t.Fatal("lost incident")
	}
	input(t, a, record, 4, 200)
	if !state(t, a).Open {
		t.Fatal("premature recovery")
	}
	input(t, a, record, 5, 200)
	if state(t, a).Open {
		t.Fatal("did not recover")
	}
	i, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Deliveries) != 2 {
		t.Fatal("lost or duplicated intents")
	}
	stale := record
	stale.Generation++
	if _, err := a.Accept(ctx, stale, source.Batch{Observations: []watch.Observation{{Health: "unknown"}}}, "stale", start); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
}
func TestApplyIsAtomicAndDryRun(t *testing.T) {
	a, _ := setup(t)
	result, err := a.Apply(ctx, ApplyRequest{Manifest: manifest("https://example.com"), DryRun: true})
	if err != nil || len(result.Changes) != 1 {
		t.Fatal(result, err)
	}
	list, err := a.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatal("dry run wrote", list, err)
	}
	broken := strings.Replace(manifest("https://example.com"), "ref: hook", "ref: missing", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: broken}); err == nil {
		t.Fatal("missing destination accepted")
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error { _, err := tx.Destination("hook", ""); return err }); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("partial destination apply", err)
	}
	record := apply(t, a, "https://example.com")
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest("https://example.com"), Expected: map[string]string{"api": "bad"}}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	result, err = a.Apply(ctx, ApplyRequest{Manifest: manifest("https://example.com"), Expected: map[string]string{"api": record.Plan.Revision}})
	if err != nil || result.Changes[0].State != "preserved" {
		t.Fatal(result, err)
	}
	for _, bad := range []string{"bad", strings.Replace(manifest("https://example.com"), "type: webhook", "type: slack", 1), strings.Replace(manifest("https://example.com"), "type: http, url: \"https://example.com\", every: 5s, timeout: 2s", "type: push", 1)} {
		if _, err := a.Apply(ctx, ApplyRequest{Manifest: bad}); err == nil {
			t.Fatal("unsupported apply accepted")
		}
	}
}
func TestDeliveryRetryOrderingAndPinnedRevision(t *testing.T) {
	a, _ := setup(t)
	record := apply(t, a, "https://example.com")
	for n, status := range []int{500, 500, 500, 200, 200} {
		input(t, a, record, n+1, status)
	}
	now := start.Add(30 * time.Second)
	a.Now = func() time.Time { return now }
	var received []string
	attempts := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		received = append(received, r.Header.Get("Idempotency-Key"))
		if attempts == 1 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	if worked, err := a.DeliverOne(ctx); err != nil || !worked {
		t.Fatal(worked, err)
	}
	i, _ := a.Inspect(ctx, "api")
	firing := i.Deliveries[1]
	if firing.Status != "pending" || firing.Attempts != 1 || firing.NextAt.Before(now.Add(time.Minute)) {
		t.Fatal(firing)
	}
	if worked, err := a.DeliverOne(ctx); err != nil || worked {
		t.Fatal("recovery overtook firing", worked, err)
	}
	now = now.Add(time.Minute)
	if _, err := a.DeliverOne(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeliverOne(ctx); err != nil {
		t.Fatal(err)
	}
	if len(received) != 3 || received[0] != received[1] || received[1] == received[2] {
		t.Fatal(received)
	}
	i, _ = a.Inspect(ctx, "api")
	for _, job := range i.Deliveries {
		if job.Status != "delivered" {
			t.Fatal(job)
		}
	}
}
func TestDeliveryPermanentExhaustionAndLostAck(t *testing.T) {
	for _, mode := range []string{"missing", "expired", "attempts", "retry_expiry", "lease", "console"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := setup(t)
			m := manifest("https://example.com")
			if mode == "console" {
				m = strings.Replace(m, "type: webhook, urlRef: {env: WEBHOOK_URL}", "type: console", 1)
			}
			if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
				t.Fatal(err)
			}
			i, _ := a.Inspect(ctx, "api")
			record := i.Watch
			for n := 1; n <= 3; n++ {
				input(t, a, record, n, 500)
			}
			now := start.Add(20 * time.Second)
			a.Now = func() time.Time { return now }
			a.Lookup = func(string) (string, bool) { return "", false }
			var output bytes.Buffer
			a.Output = &output
			if mode == "expired" {
				now = now.Add(25 * time.Hour)
			}
			if mode == "attempts" {
				for n := 0; n < 8; n++ {
					job, err := a.Store.Claim(ctx, now, time.Second)
					if err != nil || job == nil {
						t.Fatal(job, err)
					}
					now = now.Add(time.Second)
				}
			}
			if mode == "retry_expiry" {
				receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "90000")
					w.WriteHeader(429)
				}))
				defer receiver.Close()
				a.Lookup = func(string) (string, bool) { return receiver.URL, true }
			}
			if mode == "lease" {
				old, err := a.Store.Claim(ctx, now, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				now = now.Add(time.Second)
				new, err := a.Store.Claim(ctx, now, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				if new.EventID != old.EventID {
					t.Fatal("lease changed event")
				}
				if err := a.Store.Finish(ctx, *old, delivery.Result{Outcome: delivery.Delivered}, now, now); !errors.Is(err, store.ErrStale) {
					t.Fatal(err)
				}
				return
			}
			if _, err := a.DeliverOne(ctx); err != nil {
				t.Fatal(err)
			}
			i, _ = a.Inspect(ctx, "api")
			want := "exhausted"
			if mode == "missing" {
				want = "permanent"
			}
			if mode == "console" {
				want = "delivered"
				if !strings.Contains(output.String(), "firing") {
					t.Fatal(output.String())
				}
			}
			if i.Deliveries[0].Status != want {
				t.Fatal(i.Deliveries[0])
			}
		})
	}
}

func TestRuntimeChild(t *testing.T) {
	if os.Getenv("DING_RUNTIME_CHILD") != "1" {
		return
	}
	s, err := store.Open(ctx, os.Getenv("DING_RUNTIME_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if path := os.Getenv("DING_RUNTIME_STOP"); path != "" {
		go func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-childCtx.Done():
					return
				case <-ticker.C:
					if _, err := os.Stat(path); err == nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	if err := a.Run(childCtx); err != nil {
		t.Fatal(err)
	}
}

// This exercises actual five-second polling, process death with a committed
// outbox, and the same binary reopening the store. It is part of the normal gate.
func TestFiveSecondRestartDemonstration(t *testing.T) {
	a, dir := setup(t)
	var calls atomic.Int64
	var healthy atomic.Bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if healthy.Load() {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer endpoint.Close()
	var accepted atomic.Int64
	var reject atomic.Bool
	reject.Store(true)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reject.Load() {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
		} else {
			accepted.Add(1)
			w.WriteHeader(204)
		}
	}))
	defer receiver.Close()
	a.Now = func() time.Time { return time.Now().UTC() }
	apply(t, a, endpoint.URL)
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	childLogs := map[*exec.Cmd]*bytes.Buffer{}
	launch := func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeChild$")
		cmd.Env = append(os.Environ(), "DING_RUNTIME_CHILD=1", "DING_RUNTIME_DIR="+dir, "DING_RUNTIME_STOP="+filepath.Join(dir, "stop"), "WEBHOOK_URL="+receiver.URL)
		logs := &bytes.Buffer{}
		childLogs[cmd] = logs
		cmd.Stdout = logs
		cmd.Stderr = logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	child := launch()
	t.Cleanup(func() {
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
		if t.Failed() {
			for _, logs := range childLogs {
				t.Log(logs.String())
			}
		}
	})
	deadline := time.Now().Add(18 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	// Allow the third response to commit and the first attempt to see 429.
	time.Sleep(500 * time.Millisecond)
	if calls.Load() < 3 {
		t.Fatal("did not poll every five seconds")
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	child = nil
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a = New(s)
	i, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !state(t, a).Open || len(i.Deliveries) != 1 || i.Deliveries[0].Status != "pending" || i.Deliveries[0].Attempts < 1 {
		t.Fatalf("lost firing/retry: %+v", i)
	}
	eventID := i.Deliveries[0].EventID
	s.Close()
	healthy.Store(true)
	reject.Store(false)
	child = launch()
	deadline = time.Now().Add(18 * time.Second)
	for accepted.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "stop"), []byte("stop"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- child.Wait() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("child failed graceful shutdown")
	}
	child = nil
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	i, err = a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Load() != 2 || state(t, a).Open || len(i.Deliveries) != 2 || i.Deliveries[1].EventID != eventID {
		t.Fatalf("restart demo failed: accepted %d %+v", accepted.Load(), i)
	}
	for _, job := range i.Deliveries {
		if job.Status != "delivered" {
			t.Fatal(job)
		}
	}
	t.Logf("five-second restart demonstration: %d polls, firing %s, two delivered transitions; database %s", calls.Load(), eventID, filepath.Base(dir))
}

func TestRunBoundsAcquisitionAndShutsDown(t *testing.T) {
	a, _ := setup(t)
	var active, maxActive, calls atomic.Int64
	observed := make(chan struct{}, 4)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		calls.Add(1)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(1100 * time.Millisecond):
		}
		w.WriteHeader(200)
		observed <- struct{}{}
	}))
	defer endpoint.Close()
	a.Now = func() time.Time { return time.Now().UTC() }
	m := strings.Replace(manifest(endpoint.URL), "every: 5s", "every: 1s", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Run(runCtx) }()
	for i := 0; i < 2; i++ {
		select {
		case <-observed:
		case <-time.After(6 * time.Second):
			t.Fatal("poll did not finish")
		}
	}
	if err := a.Run(ctx); err == nil {
		t.Fatal("two schedulers accepted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("shutdown hung")
	}
	if maxActive.Load() != 1 || calls.Load() != 2 {
		t.Fatal("overlapping polls", maxActive.Load(), calls.Load())
	}
	a.AcquisitionWorkers = 0
	if err := a.Run(ctx); err == nil {
		t.Fatal("invalid workers accepted")
	}
}
func TestRunDrainsCommittedDelivery(t *testing.T) {
	a, _ := setup(t)
	record := apply(t, a, "http://127.0.0.1:1")
	for n := 1; n <= 3; n++ {
		input(t, a, record, n, 500)
	}
	a.Now = func() time.Time { return start.Add(20 * time.Second) }
	var accepted atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { accepted.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	runCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.Run(runCtx); err != nil {
		t.Fatal(err)
	}
	if accepted.Load() != 1 {
		t.Fatal("did not drain")
	}
	i, _ := a.Inspect(ctx, "api")
	if i.Deliveries[0].Status != "delivered" {
		t.Fatal(i.Deliveries[0])
	}
}
