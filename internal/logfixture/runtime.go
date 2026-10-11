package logfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

type Runtime struct {
	app                 *watchrun.App
	client              control.Client
	session             watchrun.LogSession
	sequence, lastEvent int64
	stop                context.CancelFunc
	done                chan error
	server, poll        *httptest.Server
	mu                  sync.Mutex
	metrics             RuntimeMetrics
}

type RuntimeMetrics struct {
	Summaries       int            `json:"summaries"`
	MaxCommitMS     float64        `json:"maxCommitMS"`
	Events          map[string]int `json:"events"`
	FiringCommitted []time.Time    `json:"firingCommitted"`
	FiringDelivered []time.Time    `json:"firingDelivered"`
	PollingHealthy  bool           `json:"pollingHealthy"`
	Usage           store.Usage    `json:"usage"`
}

func NewRuntime(ctx context.Context, dir, format string) (_ *Runtime, err error) {
	s, err := store.Open(ctx, dir)
	if err != nil {
		return nil, err
	}
	r := &Runtime{app: watchrun.New(s), metrics: RuntimeMetrics{Events: map[string]int{}, FiringCommitted: []time.Time{}, FiringDelivered: []time.Time{}}}
	defer func() {
		if err != nil {
			r.Close()
		}
	}()
	r.app.Output = r
	r.poll = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"ok":true}`) }))
	manifest := fmt.Sprintf(`apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: counting}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: workload}
spec:
  source: {type: logs, logs: {format: %s, window: 1s, minEvents: 1, retention: 1h}}
  condition: {field: warning_match_ratio, operator: gt, value: 0.5}
  policy: {recoverAfter: 1}
  destinations: [{ref: counting, events: [firing, recovered]}]
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: concurrent-http}
spec:
  source: {type: http, url: %q, every: 1s}
  condition: {field: http.status, operator: gte, value: 500}
`, format, r.poll.URL)
	if _, err = r.app.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		return nil, err
	}
	credentials, err := control.PrivateCredentials(dir)
	if err != nil {
		return nil, err
	}
	r.server = httptest.NewServer(control.Handler(r.app, credentials))
	r.client = control.Client{URL: r.server.URL, Token: credentials.Ingest}
	data, err := r.client.Call(ctx, http.MethodPost, "/v1/logs/workload/session", nil)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &r.session); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.stop, r.done = cancel, make(chan error, 1)
	go func() { r.done <- r.app.Run(runCtx) }()
	return r, nil
}

func (r *Runtime) Write(data []byte) (int, error) {
	var event struct{ Data watch.Event }
	if err := json.Unmarshal(data, &event); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if event.Data.Type == "firing" {
		r.metrics.FiringDelivered = append(r.metrics.FiringDelivered, time.Now())
	}
	return len(data), nil
}

func (r *Runtime) Publish(ctx context.Context, summary logs.Summary, ended bool) error {
	r.sequence++
	attempt, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := r.client.Produce(attempt, "/v1/logs/workload/summary", fmt.Sprint(r.sequence), watchrun.LogSnapshot{Session: r.session.ID, Sequence: r.sequence, Summary: summary, Ended: ended})
	committed := time.Now()
	if err != nil {
		return err
	}
	var events []watch.Event
	err = r.app.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		events, err = tx.Events("workload", r.lastEvent, 100)
		return err
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.Summaries++
	r.metrics.MaxCommitMS = max(r.metrics.MaxCommitMS, float64(committed.Sub(start))/float64(time.Millisecond))
	for _, e := range events {
		r.lastEvent = max(r.lastEvent, e.Sequence)
		r.metrics.Events[e.Type]++
		if e.Type == "firing" {
			r.metrics.FiringCommitted = append(r.metrics.FiringCommitted, committed)
		}
	}
	return nil
}

func (r *Runtime) Metrics(ctx context.Context) (RuntimeMetrics, error) {
	var usage store.Usage
	err := r.app.Store.View(ctx, func(tx *store.Tx) error { var err error; usage, err = tx.Usage(); return err })
	if err != nil {
		return RuntimeMetrics{}, err
	}
	poll, err := r.app.Record(ctx, "concurrent-http")
	if err != nil {
		return RuntimeMetrics{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.metrics
	m.Usage, m.PollingHealthy = usage, poll.LastError == "" && time.Since(poll.LastInputAt) < 3*time.Second
	return m, nil
}

func (r *Runtime) Close() {
	if r.stop != nil {
		r.stop()
		<-r.done
	}
	if r.server != nil {
		r.server.Close()
	}
	if r.poll != nil {
		r.poll.Close()
	}
	r.app.Store.Close()
}
