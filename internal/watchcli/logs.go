package watchcli

import (
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
		cmd.Flags().StringVar(&id, "watch", "", "existing active push watch receiving log summaries")
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
			collector, err := logs.NewCollector(parser, window)
			if err != nil {
				return err
			}
			defer collector.Close()
			dir, _ := cmd.Flags().GetString("state-dir")
			client, err := control.ConnectIngest(dir)
			if err != nil {
				return err
			}
			if _, err := client.Call(cmd.Context(), http.MethodGet, "/v1/ingest/"+url.PathEscape(id)+"/ready", nil); err != nil {
				return err
			}
			mu := &sync.Mutex{}
			out, diagnostics := lockedLogOutput{mu, cmd.OutOrStdout()}, lockedLogOutput{mu, cmd.ErrOrStderr()}
			finish := publishLogs(cmd.Context(), client, id, collector, diagnostics)
			defer finish()
			if mode == "exec" {
				signals := make(chan os.Signal, 2)
				signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
				defer signal.Stop(signals)
				r := collector.Exec(cmd.Context(), logs.ExecOptions{Argv: args, Stdin: cmd.InOrStdin(), Stdout: out, Stderr: diagnostics, Signals: signals})
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
			_, err = io.Copy(io.MultiWriter(out, frame), cmd.InOrStdin())
			return err
		}
		root.AddCommand(cmd)
	}
	return root
}
