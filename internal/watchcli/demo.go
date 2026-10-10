package watchcli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

func demoCommand() *cobra.Command {
	var desktop bool
	cmd := &cobra.Command{Use: "demo", Short: "Run a labeled one-minute local alert demo, then remove its temporary state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer cancel()
		fmt.Fprintln(cmd.OutOrStdout(), "DEMONSTRATION: temporary localhost fixture, not monitoring your service. Healthy → failing → recovered. Your existing watches are untouched.")
		return runDemo(ctx, cmd, desktop, 20*time.Second)
	}}
	cmd.Flags().BoolVar(&desktop, "desktop", false, "request desktop permission and deliver the demo alerts as native notifications")
	return cmd
}

func runDemo(ctx context.Context, cmd *cobra.Command, desktop bool, phase time.Duration) error {
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
		if err := notify.Send(ctx, notify.Message{ID: "ding-demo-test", Title: "Ding demonstration", Body: "This is a temporary demo. A failure and recovery will follow.", RequestPermission: true}); err != nil {
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
	app.Output = cmd.OutOrStdout()
	app.Notify = notify.Send
	app.AcquisitionWorkers, app.DeliveryWorkers = 1, 1
	if _, err := app.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		return err
	}
	runningCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- app.Run(runningCtx) }()
	defer func() { stop(); <-done }()
	for _, state := range []bool{false, true, false} {
		failing.Store(state)
		label := "healthy"
		if state {
			label = "failing (HTTP 503)"
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Demo fixture:", label)
		timer := time.NewTimer(phase)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	var events []watch.Event
	err = app.Store.View(ctx, func(tx *store.Tx) error { var err error; events, err = tx.Events("demo", 0, 100); return err })
	if err != nil {
		return err
	}
	fired, recovered := false, false
	for _, event := range events {
		fired = fired || event.Type == "firing"
		recovered = recovered || event.Type == "recovered"
	}
	if !fired || !recovered {
		return fmt.Errorf("demo did not record both transitions; it is not a successful alert-pipeline test")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Demo recorded a firing and a recovery. Temporary fixture and watch state are being removed. Create a real watch with ding setup or ding watch create.")
	return nil
}
