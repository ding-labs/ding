package watchrun

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
)

type blockedWriter struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	if w.calls.Add(1) == 1 {
		close(w.entered)
	}
	<-w.release
	return len(p), nil
}
func TestBlockedConsoleHasOneOutstandingWriteAndBoundedShutdown(t *testing.T) {
	a, _ := setup(t)
	writer := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	a.Output = writer
	defer close(writer.release)
	for n := 0; n < 3; n++ {
		attempt, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		result := a.console(attempt, []byte(`{"event":"blocked"}`))
		cancel()
		if result.Outcome != delivery.Retryable {
			t.Fatal(result)
		}
	}
	if writer.calls.Load() != 1 {
		t.Fatal("unbounded blocked writes", writer.calls.Load())
	}
	m := strings.Replace(manifest("https://example.com"), "type: webhook, urlRef: {env: WEBHOOK_URL}", "type: console", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Record(ctx, "api")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	a.Now = func() time.Time { return start.Add(time.Minute) }
	runCtx, cancel := context.WithCancel(ctx)
	cancel()
	done := make(chan error, 1)
	started := time.Now()
	go func() { done <- a.Run(runCtx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("blocked console prevented shutdown")
	}
	if time.Since(started) > 6*time.Second {
		t.Fatal("drain exceeded bound")
	}
	i, err := a.Inspect(ctx, "api")
	if err != nil || len(i.Deliveries) != 1 || i.Deliveries[0].Status != "pending" {
		t.Fatal(i, err)
	}
	if writer.calls.Load() != 1 {
		t.Fatal("shutdown launched another blocked write")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestConsoleShortWriteIsNotSuccess(t *testing.T) {
	a, _ := setup(t)
	a.Output = shortWriter{}
	if r := a.console(ctx, []byte("event")); r.Outcome != delivery.Retryable {
		t.Fatal(r)
	}
}
