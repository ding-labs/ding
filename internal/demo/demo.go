// Package demo runs a disposable real HTTP watch through failure and recovery.
package demo

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

type Update struct {
	Stage string       `json:"stage,omitempty"`
	Event *watch.Event `json:"event,omitempty"`
	Error string       `json:"error,omitempty"`
}

type Options struct {
	Desktop  bool
	Phase    time.Duration
	Output   io.Writer
	Progress func(Update) error
}

func Run(ctx context.Context, options Options) error {
	phase := options.Phase
	if phase == 0 {
		phase = 20 * time.Second
	}
	if phase < time.Second {
		return fmt.Errorf("demo phase must be at least one second")
	}
	desktop := options.Desktop
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	emit := func(update Update) error {
		if options.Progress != nil {
			return options.Progress(update)
		}
		return nil
	}
	// Progress and delivery workers share one stream, which may be a buffer or
	// another writer that does not support concurrent writes.
	out := &demoWriter{Writer: output}
	dir, err := os.MkdirTemp("", "ding-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := store.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	var failing atomic.Bool
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}), ReadHeaderTimeout: time.Second}
	go server.Serve(listener)
	defer server.Close()
	delivery := "console"
	if desktop {
		permissionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := notify.Send(permissionCtx, notify.Message{ID: "ding-demo-test", Title: "Ding demonstration", Body: "This is a temporary demo. A failure and recovery will follow.", RequestPermission: true})
		cancel()
		if err != nil {
			return err
		}
		delivery = "desktop"
	}
	manifest, _, err := onboarding.Manifest(onboarding.Request{ID: "demo", URL: "http://" + listener.Addr().String(), Delivery: delivery})
	if err != nil {
		return err
	}
	manifest = strings.ReplaceAll(manifest, "every: 30s", "every: "+(phase/5).String())
	app := watchrun.New(db)
	app.Output = out
	app.Notify = notify.Send
	app.AcquisitionWorkers, app.DeliveryWorkers = 1, 1
	if _, err := app.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		return err
	}
	runningCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	var runErr error
	go func() { runErr = app.Run(runningCtx); close(done) }()
	defer func() { stop(); <-done }()
	var cursor int64
	fired, recovered := false, false
	collect := func() error {
		var events []watch.Event
		err := app.Store.View(ctx, func(tx *store.Tx) error { var err error; events, err = tx.Events("demo", cursor, 100); return err })
		if err != nil {
			return err
		}
		for _, event := range events {
			cursor = event.Sequence
			if event.Type != "firing" && event.Type != "recovered" {
				continue
			}
			fired = fired || event.Type == "firing"
			recovered = recovered || event.Type == "recovered"
			if err := emit(Update{Event: &event}); err != nil {
				return err
			}
		}
		return nil
	}
	for index, state := range []bool{false, true, false} {
		failing.Store(state)
		label := "healthy"
		if state {
			label = "failing (HTTP 503)"
		}
		fmt.Fprintln(out, "Demo fixture:", label)
		stage := []string{"healthy", "failing", "recovering"}[index]
		if err := emit(Update{Stage: stage}); err != nil {
			return err
		}
		deadline := time.NewTimer(phase)
		tick := time.NewTicker(phase / 10)
		phaseErr := func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-done:
					return fmt.Errorf("demo runtime stopped: %v", runErr)
				case <-tick.C:
					if err := collect(); err != nil {
						return err
					}
				case <-deadline.C:
					return collect()
				}
			}
		}()
		deadline.Stop()
		tick.Stop()
		if phaseErr != nil {
			return phaseErr
		}
	}
	if !fired || !recovered {
		return fmt.Errorf("demo did not record both transitions; it is not a successful alert-pipeline test")
	}
	return nil
}

type demoWriter struct {
	sync.Mutex
	io.Writer
}

func (w *demoWriter) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	return w.Writer.Write(p)
}
