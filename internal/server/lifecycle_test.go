package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/config"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
)

type blockingNotifier struct {
	entered, release chan struct{}
	stopped          atomic.Bool
	invalid          atomic.Bool
	once             sync.Once
}

func (n *blockingNotifier) Send(evaluator.Alert) error {
	n.once.Do(func() { close(n.entered) })
	<-n.release
	if n.stopped.Load() {
		n.invalid.Store(true)
	}
	return nil
}
func (n *blockingNotifier) Drain(time.Duration) { n.stopped.Store(true) }
func lifecycleEngine(t *testing.T) *evaluator.Engine {
	t.Helper()
	e, err := evaluator.NewEngine([]evaluator.EngineRule{{Name: "r", Condition: "value > 1", Cooldown: time.Hour, Alerts: []string{"n"}}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestLifecycleWaitsForDispatchAndCopiesLiveState(t *testing.T) {
	n := &blockingNotifier{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := &config.Config{Server: config.ServerConfig{Format: "json", MaxBodyBytes: 1024, DrainTimeout: config.Duration{Duration: time.Second}}}
	old := lifecycleEngine(t)
	next := lifecycleEngine(t)
	s := New(old, map[string]notifier.Notifier{"n": n}, cfg, "", nil, nil, nil)
	ingestDone := make(chan struct{})
	go func() { defer close(ingestDone); s.IngestLine([]byte(`{"metric":"cpu","value":2}`)) }()
	select {
	case <-n.entered:
	case <-time.After(time.Second):
		t.Fatal("ingest never dispatched")
	}
	swapDone := make(chan error, 1)
	go func() {
		swapDone <- s.SwapEngine(next, cfg, map[string]notifier.Notifier{"n": notifier.NewStdoutNotifier(io.Discard)}, nil, nil)
	}()
	select {
	case err := <-swapDone:
		t.Fatalf("swap overtook dispatch: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(n.release)
	<-ingestDone
	if err := <-swapDone; err != nil {
		t.Fatal(err)
	}
	if n.invalid.Load() || !n.stopped.Load() {
		t.Fatal("old resource lifetime")
	}
	if len(evaluator.SnapshotEngine(next).Cooldowns) != 1 {
		t.Fatal("live cooldown lost")
	}
	s.Close(time.Second)
	s.Close(time.Second)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"metric":"cpu","value":2}`)))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal(w.Code)
	}
	if err := s.SwapEngine(lifecycleEngine(t), cfg, nil, nil, nil); err == nil {
		t.Fatal("swap after close")
	}
	if err := s.Reload(); err == nil {
		t.Fatal("reload after close")
	}
}
func TestLifecycleConcurrentIngestSwapClose(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Format: "json", MaxBodyBytes: 1024}}
	ns := func() map[string]notifier.Notifier {
		return map[string]notifier.Notifier{"n": notifier.NewStdoutNotifier(io.Discard)}
	}
	s := New(lifecycleEngine(t), ns(), cfg, "", nil, nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() { s.IngestLine([]byte(`{"metric":"cpu","value":2}`)) })
		e := lifecycleEngine(t)
		wg.Go(func() { _ = s.SwapEngine(e, cfg, ns(), nil, nil) })
	}
	wg.Go(func() { s.Close(time.Second) })
	wg.Wait()
}
