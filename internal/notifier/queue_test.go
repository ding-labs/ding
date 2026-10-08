package notifier

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/metrics"
)

func TestQueueBoundAndCancellation(t *testing.T) {
	n := newQueuedNotifier(3, time.Millisecond, nil)
	started := make(chan struct{})
	if err := n.enqueue("r", func(ctx context.Context) delivery.Result {
		close(started)
		<-ctx.Done()
		return delivery.Result{Outcome: delivery.Retryable}
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	for i := 0; i < 255; i++ {
		if err := n.enqueue("r", func(context.Context) delivery.Result { return delivery.Result{Outcome: delivery.Delivered} }); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.enqueue("r", nil); err != ErrQueueFull {
		t.Fatal(err)
	}
	n.Drain(time.Millisecond)
	select {
	case <-n.done:
	case <-time.After(time.Second):
		t.Fatal("worker leak")
	}
	if n.QueueDepth() != 0 {
		t.Fatal(n.QueueDepth())
	}
	if err := n.enqueue("r", nil); err != ErrStopped {
		t.Fatal(err)
	}
}
func TestQueueConcurrentDrainAdmission(t *testing.T) {
	for i := 0; i < 50; i++ {
		n := newQueuedNotifier(1, time.Millisecond, nil)
		var wg sync.WaitGroup
		for j := 0; j < 20; j++ {
			wg.Go(func() {
				_ = n.enqueue("r", func(context.Context) delivery.Result { return delivery.Result{Outcome: delivery.Delivered} })
			})
		}
		wg.Go(func() { n.Drain(time.Second) })
		wg.Wait()
		n.Stop()
		select {
		case <-n.done:
		case <-time.After(time.Second):
			t.Fatal("worker leak")
		}
		if n.QueueDepth() != 0 {
			t.Fatal("lost completion")
		}
	}
}
func TestQueueRetryDeadlineAndMetrics(t *testing.T) {
	for _, outcome := range []delivery.Outcome{delivery.Permanent, delivery.Retryable, delivery.Delivered} {
		t.Run(string(outcome), func(t *testing.T) {
			c := metrics.NewCollector()
			n := newQueuedNotifier(2, time.Millisecond, c)
			var calls atomic.Int32
			first := time.Now()
			_ = n.enqueue("r", func(context.Context) delivery.Result {
				calls.Add(1)
				return delivery.Result{Outcome: outcome, RetryAt: first.Add(30 * time.Millisecond)}
			})
			n.Drain(time.Second)
			if outcome == delivery.Retryable && (calls.Load() != 2 || time.Since(first) < 30*time.Millisecond) {
				t.Fatal("retry deadline/attempts", calls.Load())
			}
			if outcome != delivery.Retryable && calls.Load() != 1 {
				t.Fatal("unexpected retry")
			}
			var b bytes.Buffer
			c.WritePrometheus(&b, 0)
			want := `result="success"} 0`
			if outcome == delivery.Delivered {
				want = `result="success"} 1`
			}
			if !bytes.Contains(b.Bytes(), []byte(want)) {
				t.Fatal(b.String())
			}
		})
	}
}
func TestDueRetryDoesNotBlockNewJob(t *testing.T) {
	n := newQueuedNotifier(2, time.Hour, nil)
	defer n.Stop()
	attempted := make(chan struct{})
	done := make(chan struct{})
	_ = n.enqueue("slow", func(context.Context) delivery.Result {
		close(attempted)
		return delivery.Result{Outcome: delivery.Retryable}
	})
	<-attempted
	_ = n.enqueue("fast", func(context.Context) delivery.Result {
		close(done)
		return delivery.Result{Outcome: delivery.Delivered}
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retry blocks fresh work")
	}
}
