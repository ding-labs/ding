package watchrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

const maxLogReaders = 8

type logWorker struct {
	generation int64
	cancel     context.CancelFunc
	done       chan struct{}
	reconnect  time.Duration
}

type logWorkers struct {
	app       *App
	active    map[string]logWorker
	retry     map[string]time.Time
	completed chan<- struct{}
}

func (w *logWorkers) reconcile(ctx context.Context, records []store.WatchRecord, now time.Time) {
	desired := map[string]store.WatchRecord{}
	for _, r := range records {
		s := r.Plan.Definition.Spec.Source
		if r.Status == "running" && s.Type == "logs" && s.Logs.Input.Type != "external" {
			desired[r.Plan.Definition.Metadata.ID] = r
		}
	}
	for id, worker := range w.active {
		if r, ok := desired[id]; !ok || r.Generation != worker.generation {
			worker.cancel()
		}
		select {
		case <-worker.done:
			delete(w.active, id)
			w.retry[id] = now.Add(worker.reconnect)
		default:
		}
	}
	for id := range w.retry {
		if _, ok := desired[id]; !ok {
			delete(w.retry, id)
		}
	}
	for _, r := range records {
		id := r.Plan.Definition.Metadata.ID
		if _, ok := desired[id]; !ok {
			continue
		}
		if _, ok := w.active[id]; ok || w.retry[id].After(now) {
			continue
		}
		if len(w.active) >= maxLogReaders {
			var c logCheckpoint
			json.Unmarshal([]byte(r.Cursor), &c)
			if c.Status != "capacity" || c.Generation != r.Generation {
				c.Status, c.Generation = "capacity", r.Generation
				key, _ := watch.Revision(r.Cursor)
				_, err := w.app.accept(ctx, r, source.Batch{ExpectedCursor: &r.Cursor, Cursor: c.encode(), Observations: []watch.Observation{{Health: "unknown", Detail: "collector_capacity", Fields: map[string]any{"reason": "collector_capacity", "source_status": "capacity"}}}}, fmt.Sprintf("log-capacity:%d:%s", r.Generation, key), w.app.Now)
				w.app.note(err)
			}
			continue
		}
		readerCtx, cancel := context.WithCancel(ctx)
		delay, _ := time.ParseDuration(r.Plan.Definition.Spec.Source.Logs.Input.ReconnectEvery)
		if delay < 5*time.Second {
			delay = 5 * time.Second
		}
		worker := logWorker{generation: r.Generation, cancel: cancel, done: make(chan struct{}), reconnect: delay}
		w.active[id] = worker
		delete(w.retry, id)
		w.app.mu.Lock()
		w.app.cancels[id] = acquisition{r.Generation, cancel}
		w.app.mu.Unlock()
		go func() {
			defer func() {
				close(worker.done)
				select {
				case w.completed <- struct{}{}:
				default:
				}
			}()
			defer cancel()
			defer func() { w.app.mu.Lock(); delete(w.app.cancels, id); w.app.mu.Unlock() }()
			w.app.runLogReader(readerCtx, r)
		}()
	}
}

func (w *logWorkers) stop() {
	for _, worker := range w.active {
		worker.cancel()
	}
	for _, worker := range w.active {
		<-worker.done
	}
}

// The scheduler owns retry and active; worker completion only sends a hint.
func (w *logWorkers) nextDeadline(now time.Time) time.Time {
	var next time.Time
	for _, due := range w.retry {
		if due.After(now) && (next.IsZero() || due.Before(next)) {
			next = due
		}
	}
	return next
}
