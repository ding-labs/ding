package watchcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

type lockedLogOutput struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w lockedLogOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func logsCommand() *cobra.Command {
	root := &cobra.Command{Use: "logs", Short: "Monitor application stdout, stderr, or piped output"}
	for _, mode := range []string{"exec", "ingest"} {
		var id string
		parser := logs.ParserConfig{}
		window := logs.WindowConfig{}
		cmd := &cobra.Command{Use: mode, Args: cobra.NoArgs}
		if mode == "exec" {
			cmd.Use = "exec --watch ID -- COMMAND [ARGS...]"
			cmd.Short = "Launch an application, preserve its output and status, and monitor its logs"
			cmd.Args = cobra.MinimumNArgs(1)
		} else {
			cmd.Use = "ingest --watch ID"
			cmd.Short = "Monitor and forward an application's piped output until EOF"
		}
		cmd.Flags().SetInterspersed(false)
		cmd.Flags().StringVar(&id, "watch", "", "existing active log or push watch receiving summaries")
		cmd.Flags().StringVar(&parser.Format, "format", "auto", "auto, text, or jsonl")
		cmd.Flags().StringVar(&parser.LevelField, "level-field", "level", "JSON severity field, with optional dotted path")
		cmd.Flags().StringToStringVar(&parser.Patterns, "patterns", nil, "explicit severity-to-regex rules")
		cmd.Flags().DurationVar(&window.Window, "window", time.Minute, "whole-second counting window (at most one hour)")
		cmd.Flags().IntVar(&window.MinEvents, "min-events", 100, "minimum records required before evaluating a ratio")
		cmd.Flags().Float64Var(&window.MinCoverage, "min-coverage", 0, "required recognized severity fraction (0–1)")
		cmd.MarkFlagRequired("watch")
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("live log capture is currently supported on macOS and Linux")
			}
			if id == "" || len(id) > 128 {
				return fmt.Errorf("a valid watch ID is required")
			}
			dir, _ := cmd.Flags().GetString("state-dir")
			client, err := control.ConnectIngest(dir)
			if err != nil {
				return err
			}
			ready, err := client.Call(cmd.Context(), http.MethodGet, "/v1/ingest/"+url.PathEscape(id)+"/ready", nil)
			if err != nil {
				return err
			}
			var preflight struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(ready, &preflight); err != nil {
				return err
			}
			var session *watchrun.LogSession
			framing := logs.FrameConfig{}
			if preflight.Source == "logs" {
				for _, flag := range []string{"format", "level-field", "patterns", "window", "min-events", "min-coverage"} {
					if cmd.Flags().Changed(flag) {
						return fmt.Errorf("configure --%s in the native log watch manifest", flag)
					}
				}
				data, err := client.Call(cmd.Context(), http.MethodPost, "/v1/logs/"+url.PathEscape(id)+"/session", nil)
				if err != nil {
					return err
				}
				session = &watchrun.LogSession{}
				if err := json.Unmarshal(data, session); err != nil {
					return err
				}
				config := session.Config
				framing = watchrun.LogFraming(config)
				parser = logs.ParserConfig{Format: config.Format, LevelField: config.LevelField, Patterns: config.Patterns}
				duration, _ := time.ParseDuration(config.Window)
				window = logs.WindowConfig{Window: duration, MinEvents: config.MinEvents, MinCoverage: config.MinCoverage}
			}
			collector, err := logs.NewCollector(parser, window, framing)
			if err != nil {
				return err
			}
			defer collector.Close()
			mu := &sync.Mutex{}
			out, diagnostics := lockedLogOutput{mu, cmd.OutOrStdout()}, lockedLogOutput{mu, cmd.ErrOrStderr()}
			finish := publishLogs(cmd.Context(), client, id, collector, diagnostics, session)
			var exitCode *int
			defer func() { finish(exitCode) }()
			if mode == "exec" {
				signals := make(chan os.Signal, 2)
				signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
				defer signal.Stop(signals)
				r := collector.Exec(cmd.Context(), logs.ExecOptions{Argv: args, Stdin: cmd.InOrStdin(), Stdout: out, Stderr: diagnostics, Signals: signals})
				exitCode = &r.ExitCode
				if r.Err != nil {
					fmt.Fprintln(diagnostics, "ding: application capture:", r.Err)
				}
				if r.ExitCode != 0 {
					return &ExitStatus{Code: r.ExitCode}
				}
				return nil
			}
			frame := collector.Stream()
			defer frame.Close()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			stopReader := interruptLogInput(ctx, cmd.InOrStdin())
			defer stopReader()
			_, err = io.Copy(io.MultiWriter(out, frame), cmd.InOrStdin())
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		root.AddCommand(cmd)
	}
	return root
}

// Real stdin is closeable. Closing it interrupts a blocked pipe read so the
// collection session can end cleanly even when its producer is silent.
func interruptLogInput(ctx context.Context, input io.Reader) func() {
	done := make(chan struct{})
	if closer, ok := input.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				_ = closer.Close()
			case <-done:
			}
		}()
	}
	return func() { close(done) }
}
