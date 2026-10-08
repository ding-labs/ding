package notifier

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/metrics"
)

var ErrStopped = errors.New("notifier is draining or stopped")
var ErrQueueFull = errors.New("notifier queue is full")

type deliveryJob struct {
	rule     string
	attempt  func(context.Context) delivery.Result
	attempts int
	due      time.Time
}

// queuedNotifier owns all admission, scheduling, and finalization. Pending work
// is memory-only in the legacy runtime; a deadline or crash can lose deliveries.
type queuedNotifier struct {
	mu          sync.Mutex
	closed      bool
	pending     int
	idle        chan struct{}
	wake        chan struct{}
	jobs        []deliveryJob
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	maxAttempts int
	backoff     time.Duration
	collector   *metrics.Collector
}

func newQueuedNotifier(attempts int, backoff time.Duration, collector *metrics.Collector) *queuedNotifier {
	ctx, cancel := context.WithCancel(context.Background())
	idle := make(chan struct{})
	close(idle)
	n := &queuedNotifier{ctx: ctx, cancel: cancel, done: make(chan struct{}), idle: idle, wake: make(chan struct{}, 1), maxAttempts: max(1, attempts), backoff: backoff, collector: collector}
	go n.worker()
	return n
}
func (n *queuedNotifier) enqueue(rule string, attempt func(context.Context) delivery.Result) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return ErrStopped
	}
	if n.pending >= 256 {
		if n.collector != nil {
			n.collector.IncrWebhookDrop()
		}
		return ErrQueueFull
	}
	if n.pending == 0 {
		n.idle = make(chan struct{})
	}
	n.pending++
	n.jobs = append(n.jobs, deliveryJob{rule: rule, attempt: attempt, due: time.Now()})
	n.signal()
	return nil
}
func (n *queuedNotifier) signal() {
	select {
	case n.wake <- struct{}{}:
	default:
	}
}
func (n *queuedNotifier) QueueDepth() int { n.mu.Lock(); defer n.mu.Unlock(); return n.pending }
func (n *queuedNotifier) Stop()           { n.mu.Lock(); n.closed = true; n.cancel(); n.mu.Unlock() }
func (n *queuedNotifier) Drain(timeout time.Duration) {
	n.mu.Lock()
	n.closed = true
	idle := n.idle
	n.mu.Unlock()
	timer := time.NewTimer(max(timeout, 0))
	defer timer.Stop()
	select {
	case <-idle:
	case <-timer.C:
		log.Printf("ding: drain deadline; %d legacy deliveries unfinished", n.QueueDepth())
	}
	n.Stop()
}
func (n *queuedNotifier) finish(result delivery.Result) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.collector != nil {
		if result.Outcome == delivery.Delivered {
			n.collector.IncrWebhookSuccess()
		} else {
			n.collector.IncrWebhookFailed()
		}
	}
	n.pending--
	if n.pending == 0 {
		close(n.idle)
	}
}
func (n *queuedNotifier) worker() {
	defer close(n.done)
	defer func() {
		n.mu.Lock()
		jobs := n.jobs
		n.jobs = nil
		n.mu.Unlock()
		for range jobs {
			n.finish(delivery.Result{Outcome: delivery.Canceled})
		}
	}()
	for {
		if n.ctx.Err() != nil {
			return
		}
		n.mu.Lock()
		index := -1
		for i := range n.jobs {
			if index < 0 || n.jobs[i].due.Before(n.jobs[index].due) {
				index = i
			}
		}
		var job deliveryJob
		delay := time.Hour
		ready := false
		if index >= 0 {
			delay = time.Until(n.jobs[index].due)
			if delay <= 0 {
				job = n.jobs[index]
				n.jobs = append(n.jobs[:index], n.jobs[index+1:]...)
				ready = true
			}
		}
		n.mu.Unlock()
		if !ready {
			timer := time.NewTimer(max(delay, 0))
			select {
			case <-n.ctx.Done():
			case <-n.wake:
			case <-timer.C:
			}
			timer.Stop()
			continue
		}
		ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
		result := job.attempt(ctx)
		cancel()
		job.attempts++
		if n.ctx.Err() != nil {
			result.Outcome = delivery.Canceled
		}
		if result.Outcome == delivery.Retryable && job.attempts < n.maxAttempts {
			job.due = time.Now().Add(delivery.Backoff(n.backoff, job.attempts))
			if result.RetryAt.After(job.due) {
				job.due = result.RetryAt
			}
			n.mu.Lock()
			n.jobs = append(n.jobs, job)
			n.mu.Unlock()
			continue
		}
		if result.Outcome == delivery.Retryable {
			result.Outcome = delivery.Exhausted
		}
		if result.Outcome != delivery.Delivered {
			log.Printf("ding: delivery %s for rule %q after %d attempts: %s", result.Outcome, job.rule, job.attempts, result.Detail)
		}
		n.finish(result)
	}
}

// DrainAll gives all adapters the same deadline, rather than multiplying the
// shutdown allowance by the number of destinations. Call after producers stop.
func DrainAll(ns map[string]Notifier, timeout time.Duration) {
	var wg sync.WaitGroup
	for _, n := range ns {
		wg.Go(func() {
			if d, ok := n.(interface{ Drain(time.Duration) }); ok {
				d.Drain(timeout)
			} else if s, ok := n.(interface{ Stop() }); ok {
				s.Stop()
			}
		})
	}
	wg.Wait()
}
