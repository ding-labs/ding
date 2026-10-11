package watchrun

import (
	"context"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

const maxLogReaders = 8

type logWorker struct {
	generation int64
	cancel     context.CancelFunc
	done       chan struct{}
	reconnect  time.Duration
}

type logWorkers struct {
	app    *App
	active map[string]logWorker
	retry  map[string]time.Time
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
		if _, ok := w.active[id]; ok || len(w.active) >= maxLogReaders || w.retry[id].After(now) {
			continue
		}
		readerCtx, cancel := context.WithCancel(ctx)
		delay, _ := time.ParseDuration(r.Plan.Definition.Spec.Source.Logs.Input.ReconnectEvery)
		if delay < 5*time.Second {
			delay = 5 * time.Second
		}
		worker := logWorker{generation: r.Generation, cancel: cancel, done: make(chan struct{}), reconnect: delay}
		w.active[id] = worker
		w.app.mu.Lock()
		w.app.cancels[id] = acquisition{r.Generation, cancel}
		w.app.mu.Unlock()
		go func() {
			defer close(worker.done)
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
