package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"github.com/ding-labs/ding/internal/server"
)

func newServeCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the alerting daemon",
		Long: `Start the DING alerting daemon.

Loads rules from the config file, starts an HTTP server on the configured port,
and begins evaluating incoming events. Automatically detects piped stdin and
processes it alongside the HTTP server.

Signals:
  SIGHUP   — hot-reload config without restarting
  SIGTERM  — graceful shutdown (30s drain)
  SIGINT   — graceful shutdown (30s drain)

If persistence.state_file is configured, cooldown state and windowed buffers
are restored from disk on startup and flushed periodically.`,
		Example: `  # Start with default config (ding.yaml)
  ding serve

  # Point at a specific config file
  ding serve --config /etc/myapp/alerts.yaml

  # Pipe events from another process
  your-app | ding serve`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(configPath)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "ding.yaml", "path to config file")
	return cmd
}

func runServe(configPath string) error {
	// Collector is created once and never recreated — counters accumulate across hot-reloads.
	collector := metrics.NewCollector()

	eng, cfg, notifiers, alertLogger, jqCode, err := server.BuildFromConfig(configPath, collector)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	srv := server.New(eng, notifiers, cfg, configPath, collector, alertLogger, jqCode)

	// reloadMu serializes concurrent reloads from both the SIGHUP goroutine
	// and the /reload HTTP endpoint (via the reload hook closure below).
	var reloadMu sync.Mutex

	// Persistence: restore state and start periodic flusher.
	stopFlusher := func() {} // no-op until flusher is started
	if cfg.Persistence.StateFile != "" {
		snap, err := evaluator.LoadSnapshot(cfg.Persistence.StateFile)
		if err != nil {
			log.Printf("ding: could not load state: %v (starting fresh)", err)
		} else if snap != nil {
			evaluator.RestoreEngine(eng, *snap, time.Now())
			log.Printf("ding: restored state from %s (saved at %s)", cfg.Persistence.StateFile, snap.SavedAt.Format(time.RFC3339))
		}
		stopFlusher = eng.StartFlusher(cfg.Persistence.StateFile, cfg.Persistence.FlushInterval.Duration)
	}

	// Set up the reload hook so /reload endpoint also transfers state.
	srv.SetReloadHook(func() error {
		reloadMu.Lock()
		defer reloadMu.Unlock()

		// Flush old engine state to disk.
		stopFlusher()
		stopFlusher = func() {}

		newEng, newCfg, newNotifiers, newAlertLogger, newJQCode, err := server.BuildFromConfig(configPath, collector)
		if err != nil {
			// Restart flusher on old engine since reload failed.
			if cfg.Persistence.StateFile != "" {
				stopFlusher = eng.StartFlusher(cfg.Persistence.StateFile, cfg.Persistence.FlushInterval.Duration)
			}
			return fmt.Errorf("reload failed: %w", err)
		}

		// Restore state into new engine from file.
		if newCfg.Persistence.StateFile != "" {
			snap, err := evaluator.LoadSnapshot(newCfg.Persistence.StateFile)
			if err != nil {
				log.Printf("ding: state restore after reload failed: %v (new engine starts fresh)", err)
			} else if snap != nil {
				evaluator.RestoreEngine(newEng, *snap, time.Now())
			}
		}

		srv.SwapEngine(newEng, newCfg, newNotifiers, newAlertLogger, newJQCode)
		eng = newEng
		cfg = newCfg
		notifiers = newNotifiers
		alertLogger = newAlertLogger
		log.Printf("ding: config reloaded from %s", configPath)

		// Start new flusher on new engine.
		if newCfg.Persistence.StateFile != "" {
			stopFlusher = newEng.StartFlusher(newCfg.Persistence.StateFile, newCfg.Persistence.FlushInterval.Duration)
		}
		return nil
	})

	// Detect stdin pipe
	stdinInfo, err := os.Stdin.Stat()
	if err == nil && (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		go readStdin(srv, cfg.Server.Format)
	}

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      srv.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout.Duration,
		WriteTimeout: cfg.Server.WriteTimeout.Duration,
		IdleTimeout:  cfg.Server.IdleTimeout.Duration,
	}

	// Signal handling
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	go func() {
		for range sighup {
			log.Println("ding: received SIGHUP, reloading config...")
			reloadMu.Lock()

			// 1a. Flush old engine state to disk.
			stopFlusher()
			// 1b. Reset to no-op immediately so any early return path is safe.
			stopFlusher = func() {}

			newEng, newCfg, newNotifiers, newAlertLogger, newJQCode, err := server.BuildFromConfig(configPath, collector)
			if err != nil {
				log.Printf("ding: reload failed: %v (keeping current config)", err)
				// Restart flusher on old engine since reload failed.
				if cfg.Persistence.StateFile != "" {
					stopFlusher = eng.StartFlusher(cfg.Persistence.StateFile, cfg.Persistence.FlushInterval.Duration)
				}
				reloadMu.Unlock()
				continue
			}

			// Restore state into new engine from file.
			if newCfg.Persistence.StateFile != "" {
				snap, err := evaluator.LoadSnapshot(newCfg.Persistence.StateFile)
				if err != nil {
					log.Printf("ding: state restore after reload failed: %v (new engine starts fresh)", err)
				} else if snap != nil {
					evaluator.RestoreEngine(newEng, *snap, time.Now())
				}
			}

			srv.SwapEngine(newEng, newCfg, newNotifiers, newAlertLogger, newJQCode)
			eng = newEng
			cfg = newCfg
			notifiers = newNotifiers
			alertLogger = newAlertLogger
			log.Printf("ding: config reloaded from %s", configPath)

			// Start new flusher on new engine.
			if newCfg.Persistence.StateFile != "" {
				stopFlusher = newEng.StartFlusher(newCfg.Persistence.StateFile, newCfg.Persistence.FlushInterval.Duration)
			}
			reloadMu.Unlock()
		}
	}()

	log.Printf("ding: listening on :%d (config: %s)", cfg.Server.Port, configPath)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ding: server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	signal.Stop(sighup)
	close(sighup)
	log.Println("ding: shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("ding: shutdown error: %v", err)
	}

	// Stop current notifiers.
	for _, n := range notifiers {
		if stopper, ok := n.(interface{ Stop() }); ok {
			stopper.Stop()
		}
	}
	// Close the current alert logger.
	if alertLogger != nil {
		if err := alertLogger.Close(); err != nil {
			log.Printf("ding: closing alert logger: %v", err)
		}
	}
	stopFlusher()
	return nil
}

func readStdin(srv *server.Server, _ string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		srv.IngestLine(line)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("ding: stdin read error: %v", err)
	}
	// stdin EOF — HTTP server continues
}
