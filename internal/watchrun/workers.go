package watchrun

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/runtimeperf"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
)

// DeliverOne leases durably before attempting remote I/O. A failed final commit
// leaves the lease recoverable and can therefore duplicate remote receipt.
func (a *App) DeliverOne(ctx context.Context) (bool, error) {
	now := a.Now()
	claim := time.Now()
	intent, err := a.Store.Claim(ctx, now, 30*time.Second)
	runtimeperf.Since(a.Store.PerfBackend(), runtimeperf.DeliveryClaim, claim)
	if err != nil || intent == nil {
		return false, err
	}
	if intent.Attempts == 1 {
		runtimeperf.Observe(a.Store.PerfBackend(), runtimeperf.EnqueueFirst, now.Sub(intent.CreatedAt))
	}
	attempted := time.Now()
	d := intent.Destination.Definition.Spec
	maxAge, _ := time.ParseDuration(d.MaxAge)
	cycle := intent.CreatedAt
	if !intent.CycleStartedAt.IsZero() {
		cycle = intent.CycleStartedAt
	}
	deadline := cycle.Add(maxAge)
	result := delivery.Result{Outcome: delivery.Exhausted, Detail: "delivery policy exhausted"}
	if intent.Attempts <= d.MaxAttempts && now.Before(deadline) {
		if d.Type == "console" {
			attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
			result = a.console(attempt, intent.Payload)
			cancel()
		} else if d.Type == "desktop" {
			var message notify.Message
			if json.Unmarshal(intent.Payload, &message) != nil || a.Notify == nil {
				result = delivery.Result{Outcome: delivery.Permanent, Detail: "desktop_unavailable"}
			} else {
				attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := a.Notify(attempt, message)
				cancel()
				result = delivery.Result{Outcome: delivery.Delivered, Detail: "accepted_by_os"}
				if err != nil {
					result = delivery.Result{Outcome: delivery.Retryable, Detail: "desktop_unavailable_or_permission_denied"}
				}
			}
		} else {
			endpoint, err := source.Resolve(d.URLRef, a.Lookup)
			headers := http.Header{"Idempotency-Key": []string{intent.EventID}, "X-Ding-Event-Id": []string{intent.EventID}}
			for name, ref := range d.Headers {
				value, e := source.Resolve(&ref, a.Lookup)
				if e != nil {
					err = e
				}
				headers.Set(name, value)
			}
			if err != nil {
				result = delivery.Result{Outcome: delivery.Permanent, Detail: "missing_credentials"}
			} else if source.URL(endpoint) != nil {
				result = delivery.Result{Outcome: delivery.Permanent, Detail: "invalid_endpoint"}
			} else {
				attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
				client := a.HTTP.Client
				if client == nil {
					client = http.DefaultClient
				}
				result = delivery.HTTP(attempt, client, delivery.Request{URL: endpoint, Body: intent.Payload, Header: headers, Provider: d.Type}, now)
				cancel()
			}
		}
	}
	runtimeperf.Since(a.Store.PerfBackend(), runtimeperf.DeliveryIO, attempted)
	finished := a.Now()
	next := finished
	if result.Outcome == delivery.Retryable {
		initial, _ := time.ParseDuration(d.InitialBackoff)
		delay := delivery.Backoff(initial, intent.Attempts)
		// Positive jitter never violates the initial backoff or a provider deadline.
		next = finished.Add(delay + time.Duration(rand.Float64()*float64(delay)/4))
		if result.RetryAt.After(next) {
			next = result.RetryAt
		}
		if intent.Attempts >= d.MaxAttempts || !next.Before(deadline) {
			result = delivery.Result{Outcome: delivery.Exhausted, Detail: "delivery policy exhausted", RetryAt: result.RetryAt}
		}
	}
	// Cancellation of the daemon is not cancellation of durable work.
	if ctx.Err() != nil && result.Outcome != delivery.Delivered {
		result = delivery.Result{Outcome: delivery.Retryable, Detail: "shutdown_interrupted"}
		next = finished
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return true, a.Store.Finish(finishCtx, *intent, result, next, finished)
}

// Run owns bounded workers until cancellation, then stops acquisitions and
// drains committed deliveries for at most five seconds. Store ownership stays
// with the caller so its API handlers can finish before Close.
func (a *App) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("runtime already running")
	}
	a.running = true
	a.closing = false
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.running = false; a.mu.Unlock() }()
	if err := a.validateLimits(); err != nil {
		return err
	}
	if a.AcquisitionWorkers < 1 || a.DeliveryWorkers < 1 {
		return fmt.Errorf("worker counts must be positive")
	}
	retryDelay := a.PollInterval
	if retryDelay == 0 {
		retryDelay = time.Second
	}
	if retryDelay < 100*time.Millisecond || retryDelay > 5*time.Second {
		return fmt.Errorf("poll interval must be between 100ms and 5s")
	}
	if err := a.Store.LimitBytes(context.WithoutCancel(ctx), a.Limits.MaxBytes); err != nil {
		return err
	}
	deliveryCtx, stopDelivery := context.WithCancel(context.WithoutCancel(ctx))
	defer stopDelivery()
	var cached scheduleCache
	var deliveries sync.WaitGroup
	for i := 0; i < a.DeliveryWorkers; i++ {
		deliveries.Add(1)
		go func() {
			defer deliveries.Done()
			var delivery deliveryCache
			for {
				select {
				case <-deliveryCtx.Done():
					return
				default:
				}
				changed := a.Store.Changes()
				ready, err := delivery.ready(deliveryCtx, a.Store, a.Now(), &cached)
				worked := false
				if err == nil && ready {
					worked, err = a.DeliverOne(deliveryCtx)
					if err == nil && !worked {
						// Recheck the shared snapshot after a nonempty queue
						// drains; keep main's empty-queue cache between commits.
						delivery = deliveryCache{}
					}
				}
				a.note(err)
				if worked && err == nil {
					continue
				}
				next := a.Now().Add(time.Minute)
				if err == nil && ready {
					var due time.Time
					due, err = a.Store.NextDelivery(deliveryCtx)
					next = earlier(next, due)
					a.note(err)
				}
				if err != nil {
					next = a.Now().Add(retryDelay)
				}
				waitUntil(deliveryCtx, changed, nil, next.Sub(a.Now()))
			}
		}()
	}
	var acquisitions sync.WaitGroup
	active := map[string]bool{}
	retry := map[string]time.Time{}
	var activeMu sync.Mutex
	completed := make(chan struct{}, 1)
	logReaders := &logWorkers{app: a, active: map[string]logWorker{}, retry: map[string]time.Time{}, completed: completed}
	lastMaintenance := time.Time{}
	var previousWall, previousTick time.Time
	schedule := func() time.Time {
		now := a.Now()
		tickNow := time.Now()
		if !previousTick.IsZero() && clockDiscontinuity(previousWall, now, tickNow.Sub(previousTick)) {
			if err := a.clockShift(ctx, now); err != nil {
				a.note(err)
				return now.Add(retryDelay)
			}
			lastMaintenance = time.Time{}
		}
		previousWall, previousTick = now, tickNow
		if lastMaintenance.IsZero() || now.Sub(lastMaintenance) >= time.Minute {
			if err := a.maintain(ctx, now, &cached); err != nil {
				a.note(err)
				return now.Add(retryDelay)
			}
			lastMaintenance = now
		}
		state, err := cached.load(ctx, a.Store, now)
		if err != nil {
			a.note(err)
			return now.Add(retryDelay)
		}
		if state.Budget.Bytes >= a.Limits.MaxBytes || state.Budget.Pending >= a.Limits.MaxPending {
			a.note(ErrQuota)
			return now.Add(retryDelay)
		}
		if !state.TimerAt.IsZero() && !state.TimerAt.After(now) {
			cached.invalidate()
			if err := a.RunTimers(ctx, now); err != nil {
				a.note(err)
				return now.Add(retryDelay)
			}
			state, err = cached.load(ctx, a.Store, now)
			if err != nil {
				a.note(err)
				return now.Add(retryDelay)
			}
		}
		records := state.Watches
		a.expireLogSessions(ctx, records, now)
		logReaders.reconcile(ctx, records, now)
		next := earlier(lastMaintenance.Add(time.Minute), state.TimerAt)
		next = earlier(next, logReaders.nextDeadline(now))
		for _, record := range records {
			if record.Status != "running" || record.Plan.Definition.Spec.Source.Type != "logs" {
				continue
			}
			var checkpoint logCheckpoint
			if json.Unmarshal([]byte(record.Cursor), &checkpoint) == nil && checkpoint.Status == "active" {
				next = earlier(next, checkpoint.Last.Add(5*time.Second))
			}
		}

		activeMu.Lock()
		known := map[string]bool{}
		for _, record := range records {
			if record.Status == "running" {
				known[record.Plan.Definition.Metadata.ID] = true
			}
		}
		for id := range retry {
			if !known[id] {
				delete(retry, id)
			}
		}
		activeMu.Unlock()
		for _, record := range records {
			id := record.Plan.Definition.Metadata.ID
			if record.Status != "running" || (record.Plan.Definition.Spec.Source.Type == "push" || record.Plan.Definition.Spec.Source.Type == "logs") {
				continue
			}
			activeMu.Lock()
			if active[id] {
				activeMu.Unlock()
				continue
			}
			ready := record.NextAt
			if retry[id].After(ready) {
				ready = retry[id]
			}
			if ready.After(now) {
				next = earlier(next, ready)
				activeMu.Unlock()
				continue
			}
			if len(active) >= a.AcquisitionWorkers {
				activeMu.Unlock()
				continue
			}
			if err := a.Reserve(); err != nil {
				next = earlier(next, now.Add(retryDelay))
				activeMu.Unlock()
				continue
			}
			active[id] = true
			activeMu.Unlock()
			acquisitions.Add(1)
			go func(record store.WatchRecord, id string) {
				defer acquisitions.Done()
				defer func() {
					a.Release()
					activeMu.Lock()
					delete(active, id)
					activeMu.Unlock()
					select {
					case completed <- struct{}{}:
					default:
					}
				}()
				acquireCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				a.mu.Lock()
				a.cancels[id] = acquisition{record.Generation, cancel}
				a.mu.Unlock()
				defer func() { a.mu.Lock(); delete(a.cancels, id); a.mu.Unlock() }()
				// A lifecycle commit can race registration; verify the generation before I/O.
				var current store.WatchRecord
				if err := a.Store.View(acquireCtx, func(tx *store.Tx) error { var err error; current, err = tx.Watch(id); return err }); err != nil {
					// Completion wakes the scheduler immediately. Without a retry
					// deadline, a cached watch with a persistent read failure spins.
					a.note(err)
					activeMu.Lock()
					retry[id] = a.Now().Add(retryDelay)
					activeMu.Unlock()
					return
				}
				if current.Generation != record.Generation || current.Status != "running" || !current.NextAt.Equal(record.NextAt) {
					return
				}
				var acquirer source.Acquirer = a.HTTP
				if record.Plan.Definition.Spec.Source.Type == "command" {
					acquirer = source.Command{Lookup: a.Lookup}
				}
				batch := acquirer.Fetch(acquireCtx, record.Plan, record.Cursor, a.Now())

				if acquireCtx.Err() != nil {
					return
				}
				_, err := a.accept(ctx, record, batch, "poll:"+strconv.FormatInt(record.Generation, 10)+":"+strconv.FormatInt(record.NextAt.UnixNano(), 10), a.Now)
				if err != nil && err != store.ErrStale {
					a.note(err)
					activeMu.Lock()
					interval, _ := time.ParseDuration(record.Plan.Definition.Spec.Source.Every)
					if interval < time.Second {
						interval = time.Second
					}
					retry[id] = a.Now().Add(interval)
					activeMu.Unlock()
				}
			}(record, id)
		}
		return next
	}
	for ctx.Err() == nil {
		changed := a.Store.Changes()
		next := schedule()
		waitUntil(ctx, changed, completed, next.Sub(a.Now()))
	}

	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	acquisitions.Wait()
	logReaders.stop()
	drainDeadline := time.NewTimer(5 * time.Second)
	defer drainDeadline.Stop()
	for {
		changed := a.Store.Changes()
		pending := 0
		err := a.Store.View(context.Background(), func(tx *store.Tx) error { var err error; pending, err = tx.Pending(); return err })
		if err != nil || pending == 0 {
			break
		}
		select {
		case <-drainDeadline.C:
			stopDelivery()
			deliveries.Wait()
			return nil
		case <-changed:
		}
	}
	stopDelivery()
	deliveries.Wait()
	return nil
}
