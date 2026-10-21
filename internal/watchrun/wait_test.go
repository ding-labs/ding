package watchrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startRuntime(t *testing.T, a *App) {
	t.Helper()
	run, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(run) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(7 * time.Second):
			t.Error("runtime failed to shut down")
		}
	})
}

func receiveAttempt(t *testing.T, attempts <-chan time.Time) time.Time {
	t.Helper()
	select {
	case at := <-attempts:
		return at
	case <-time.After(3 * time.Second):
		t.Fatal("worker missed work/deadline")
		return time.Time{}
	}
}

func TestSleepingRuntimeWakesForWatchRetryAndRecovery(t *testing.T) {
	a, _ := setup(t)
	var nowCalls atomic.Int64
	a.Now = func() time.Time { nowCalls.Add(1); return time.Now().UTC() }
	// This is now an error retry delay, not the latency for new work.
	a.PollInterval = 5 * time.Second
	startRuntime(t, a)
	// Wait for startup to settle, then verify there are no idle polling ticks.
	deadline := time.Now().Add(2 * time.Second)
	for {
		before := nowCalls.Load()
		time.Sleep(50 * time.Millisecond)
		if before > 0 && nowCalls.Load() == before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runtime never became idle")
		}
	}
	before := nowCalls.Load()
	time.Sleep(300 * time.Millisecond)
	if nowCalls.Load() != before {
		t.Fatal("idle runtime kept polling")
	}
	var calls, status atomic.Int64
	status.Store(503)
	attempts := make(chan time.Time, 8)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts <- time.Now()
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(receiver.Close)
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(status.Load())) }))
	t.Cleanup(endpoint.Close)
	m := strings.Replace(manifest(endpoint.URL), "every: 5s", "every: 1s", 1)
	m = strings.Replace(m, "consecutive: 3, recoverAfter: 2", "consecutive: 1, recoverAfter: 1", 1)
	m = strings.Replace(m, "type: webhook,", "initialBackoff: 80ms, type: webhook,", 1)
	applied := time.Now()
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	first := receiveAttempt(t, attempts)
	if first.Sub(applied) > time.Second {
		t.Fatal("new watch/delivery waited for polling")
	}
	second := receiveAttempt(t, attempts)
	if second.Sub(first) < 80*time.Millisecond || second.Sub(first) > time.Second {
		t.Fatal("retry failed to honor scheduled backoff", second.Sub(first))
	}
	status.Store(200)
	receiveAttempt(t, attempts) // next source deadline recovers and delivers
	deadline = time.Now().Add(time.Second)
	for {
		changed := a.Store.Changes()
		i, err := a.Inspect(ctx, "api")
		if err != nil {
			t.Fatal(err)
		}
		if len(i.Deliveries) == 2 && i.Deliveries[0].Status == "delivered" && i.Deliveries[1].Status == "delivered" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("firing/recovery deliveries did not finish", i.Deliveries)
		}
		waitUntil(ctx, changed, nil, time.Until(deadline))
	}
}

func TestSleepingRuntimeHonorsMissingTimer(t *testing.T) {
	a, _ := setup(t)
	a.Now = func() time.Time { return time.Now().UTC() }
	a.PollInterval = 5 * time.Second
	attempts := make(chan time.Time, 8)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts <- time.Now()
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	m := strings.Replace(missingManifest(), "missingFor: 10s", "missingFor: 1s", 1)
	m = strings.Replace(m, `source: {type: http, url: "https://example.com", every: 5s, timeout: 2s}`, `source: {type: push}`, 1)
	created := time.Now()
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	startRuntime(t, a)
	at := receiveAttempt(t, attempts)
	if at.Sub(created) < time.Second || at.Sub(created) > 2*time.Second {
		t.Fatal("missing timer missed its deadline", at.Sub(created))
	}
}

func TestAcquisitionCompletionWakesWaitingWatch(t *testing.T) {
	a, _ := setup(t)
	a.Now = func() time.Time { return time.Now().UTC() }
	a.AcquisitionWorkers = 1
	a.PollInterval = 5 * time.Second
	fetched := make(chan time.Time, 8)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched <- time.Now()
		w.WriteHeader(200)
	}))
	defer endpoint.Close()
	m := strings.Replace(manifest(endpoint.URL), "every: 5s", "every: 1h", 1)
	for _, id := range []string{"first", "second"} {
		if _, err := a.Apply(ctx, ApplyRequest{Manifest: strings.Replace(m, "id: api", "id: "+id, 1)}); err != nil {
			t.Fatal(err)
		}
	}
	startRuntime(t, a)
	first := receiveAttempt(t, fetched)
	second := receiveAttempt(t, fetched)
	if second.Sub(first) > time.Second {
		t.Fatal("free acquisition slot did not wake scheduler")
	}
	select {
	case <-fetched:
		t.Fatal("completion triggered a duplicate acquisition")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAcquisitionReadFailureBacksOff(t *testing.T) {
	a, dir := setup(t)
	var nowCalls atomic.Int64
	a.Now = func() time.Time { nowCalls.Add(1); return time.Now().UTC() }
	fetched := make(chan time.Time, 8)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched <- time.Now()
		w.WriteHeader(200)
	}))
	defer endpoint.Close()
	m := strings.Replace(manifest(endpoint.URL), "every: 5s", "every: 1s", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	startRuntime(t, a)
	receiveAttempt(t, fetched)
	deadline := time.Now().Add(time.Second)
	for {
		changed := a.Store.Changes()
		r, err := a.Record(ctx, "api")
		if err != nil {
			t.Fatal(err)
		}
		if !r.LastInputAt.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("acquisition did not checkpoint")
		}
		waitUntil(ctx, changed, nil, time.Until(deadline))
	}
	// Corrupt the row through a separate connection, without a process-local
	// notification. The scheduler still has a valid cached plan; the acquisition
	// fence must fail without launching a rapid completion/retry loop.
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("UPDATE watches SET revision='missing-revision'"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	before := nowCalls.Load()
	time.Sleep(300 * time.Millisecond)
	if delta := nowCalls.Load() - before; delta > 20 {
		t.Fatalf("failed acquisition spun: %d clock samples in 300ms", delta)
	}
}

func TestSleepingRuntimeExpiresLogSession(t *testing.T) {
	a, _ := setup(t)
	a.Now = time.Now
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartLogSession(ctx, "output"); err != nil {
		t.Fatal(err)
	}
	startRuntime(t, a)
	deadline := time.Now().Add(7 * time.Second)
	for {
		changed := a.Store.Changes()
		r, err := a.Record(ctx, "output")
		if err != nil {
			t.Fatal(err)
		}
		var checkpoint logCheckpoint
		if err := json.Unmarshal([]byte(r.Cursor), &checkpoint); err != nil {
			t.Fatal(err)
		}
		if checkpoint.Status == "stalled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session expiry missed its deadline")
		}
		waitUntil(ctx, changed, nil, time.Until(deadline))
	}
}
