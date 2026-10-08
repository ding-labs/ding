package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	collector := metrics.NewCollector()
	eng, cfg, ns, logger, jq, err := server.BuildFromConfig(configPath, collector)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	srv := server.New(eng, ns, cfg, configPath, collector, logger, jq)
	if err := srv.StartPersistence(); err != nil {
		srv.Close(0)
		return err
	}
	defer srv.Close(cfg.Server.DrainTimeout.Duration)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	defer signal.Stop(sighup)
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				srv.Sweep(now)
			case <-sighup:
				if err := srv.Reload(); err != nil {
					log.Printf("ding: reload failed: %v", err)
				}
			}
		}
	}()
	if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
		stdinDone := make(chan struct{})
		go func() { defer close(stdinDone); readStdin(srv, cfg.Server.Format) }()
		defer func() { os.Stdin.Close(); <-stdinDone }()
	}
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: srv.Handler(), ReadTimeout: cfg.Server.ReadTimeout.Duration, WriteTimeout: cfg.Server.WriteTimeout.Duration, IdleTimeout: cfg.Server.IdleTimeout.Duration}
	result := make(chan error, 1)
	go func() { result <- httpSrv.ListenAndServe() }()
	log.Printf("ding: listening on %s", httpSrv.Addr)
	select {
	case err = <-result:
		stop()
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if shutdownErr := httpSrv.Shutdown(shutdownCtx); shutdownErr != nil {
		httpSrv.Close()
		log.Printf("ding: shutdown: %v", shutdownErr)
	}
	<-maintenanceDone
	if err != nil && err != http.ErrServerClosed {
		return err
	}
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

// An invalid snapshot must never be overwritten by a silently fresh engine.
func restoreStateFile(eng *evaluator.Engine, path string, now time.Time) error {
	return server.RestoreStateFile(eng, path, now)
}
