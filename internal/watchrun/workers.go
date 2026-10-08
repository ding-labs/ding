package watchrun

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
)

// DeliverOne leases durably before attempting remote I/O. A failed final commit
// leaves the lease recoverable and can therefore duplicate remote receipt.
func (a *App) DeliverOne(ctx context.Context) (bool, error) {
	now := a.Now()
	intent, err := a.Store.Claim(ctx, now, 30*time.Second)
	if err != nil || intent == nil {
		return false, err
	}
	d := intent.Destination.Definition.Spec
	maxAge, _ := time.ParseDuration(d.MaxAge)
	deadline := intent.CreatedAt.Add(maxAge)
	result := delivery.Result{Outcome: delivery.Exhausted, Detail: "delivery policy exhausted"}
	if intent.Attempts <= d.MaxAttempts && now.Before(deadline) {
		if d.Type == "console" {
			a.outputMu.Lock()
			_, err := fmt.Fprintln(a.Output, string(intent.Payload))
			a.outputMu.Unlock()
			if err == nil {
				result = delivery.Result{Outcome: delivery.Delivered}
			} else {
				result = delivery.Result{Outcome: delivery.Retryable, Detail: "console write failed"}
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
			result = delivery.Result{Outcome: delivery.Exhausted, Detail: "delivery policy exhausted"}
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
	if err := a.Store.LimitBytes(context.WithoutCancel(ctx), a.Limits.MaxBytes); err != nil {
		return err
	}
	deliveryCtx, stopDelivery := context.WithCancel(context.WithoutCancel(ctx))
	defer stopDelivery()
	var deliveries sync.WaitGroup
	for i := 0; i < a.DeliveryWorkers; i++ {
		deliveries.Add(1)
		go func() {
			defer deliveries.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-deliveryCtx.Done():
					return
				default:
				}
				worked, err := a.DeliverOne(deliveryCtx)
				a.note(err)
				if worked && err == nil {
					continue
				}
				select {
				case <-deliveryCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	var acquisitions sync.WaitGroup
	active := map[string]bool{}
	retry := map[string]time.Time{}
	var activeMu sync.Mutex
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	lastMaintenance := time.Time{}
	schedule := func() {
		now := a.Now()
		if lastMaintenance.IsZero() || now.Sub(lastMaintenance) >= time.Minute {
			if err := a.Maintain(ctx, now); err != nil {
				a.note(err)
				return
			}
			lastMaintenance = now
		}
		if _, err := a.Budget(ctx); err != nil {
			a.note(err)
			return
		}
		if err := a.RunTimers(ctx, now); err != nil {
			a.note(err)
			return
		}

		records, err := a.List(ctx)
		if err != nil {
			a.note(err)
			return
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
			if record.Status != "running" || record.Plan.Definition.Spec.Source.Type == "push" || record.NextAt.After(now) {
				continue
			}
			activeMu.Lock()
			if active[id] || retry[id].After(now) || len(active) >= a.AcquisitionWorkers {
				activeMu.Unlock()
				continue
			}
			if err := a.Reserve(); err != nil {
				activeMu.Unlock()
				continue
			}
			active[id] = true
			activeMu.Unlock()
			acquisitions.Add(1)
			go func(record store.WatchRecord, id string) {
				defer acquisitions.Done()
				defer a.Release()
				defer func() { activeMu.Lock(); delete(active, id); activeMu.Unlock() }()
				acquireCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				a.mu.Lock()
				a.cancels[id] = acquisition{record.Generation, cancel}
				a.mu.Unlock()
				defer func() { a.mu.Lock(); delete(a.cancels, id); a.mu.Unlock() }()
				// A lifecycle commit can race registration; verify the generation before I/O.
				var current store.WatchRecord
				if err := a.Store.View(acquireCtx, func(tx *store.Tx) error { var err error; current, err = tx.Watch(id); return err }); err != nil || current.Generation != record.Generation || current.Status != "running" {
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
				_, err := a.Accept(ctx, record, batch, "poll:"+strconv.FormatInt(record.Generation, 10)+":"+strconv.FormatInt(record.NextAt.UnixNano(), 10), a.Now())
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
	}
	if ctx.Err() == nil {
		schedule()
	}
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			if ctx.Err() == nil {
				schedule()
			}
		}
	}
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	acquisitions.Wait()
	drainDeadline := time.NewTimer(5 * time.Second)
	defer drainDeadline.Stop()
	for {
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
		case <-tick.C:
		}
	}
	stopDelivery()
	deliveries.Wait()
	return nil
}
