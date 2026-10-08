package watchcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

func runtimeCommands(root *cobra.Command) {
	base, _ := os.UserConfigDir()
	dir := filepath.Join(base, "ding", "watch")
	root.PersistentFlags().StringVar(&dir, "state-dir", dir, "directory owned by the local daemon")
	var address string
	var remote bool
	daemon := &cobra.Command{Use: "daemon", Short: "Run applied watches and durable delivery workers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := control.ValidateListen(address, remote); err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		database, err := store.Open(ctx, dir)
		if err != nil {
			return err
		}
		defer database.Close()
		credentials, err := control.PrivateCredentials(dir)
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		defer listener.Close()
		host, port, _ := net.SplitHostPort(listener.Addr().String())
		ip := net.ParseIP(host)
		if ip != nil && ip.IsUnspecified() {
			if ip.To4() != nil {
				host = "127.0.0.1"
			} else {
				host = "::1"
			}
		}
		endpoint := "http://" + net.JoinHostPort(host, port)
		if err := control.SaveConnection(dir, endpoint); err != nil {
			return err
		}
		app := watchrun.New(database)
		app.Output = cmd.OutOrStdout()
		server := &http.Server{Handler: control.Handler(app, credentials), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		served := make(chan error, 1)
		go func() { served <- server.Serve(listener) }()
		running := make(chan error, 1)
		go func() { running <- app.Run(ctx) }()
		fmt.Fprintf(cmd.ErrOrStderr(), "Ding listening on %s\n", endpoint)
		var serveErr error
		select {
		case <-ctx.Done():
		case serveErr = <-served:
			cancel()
		case err = <-running:
			cancel()
			running = nil
		}
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if e := server.Shutdown(shutdown); e != nil {
			_ = server.Close()
		}
		if running != nil {
			if e := <-running; e != nil {
				err = e
			}
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return err
	}}
	daemon.Flags().StringVar(&address, "listen", "127.0.0.1:7676", "explicit listen IP:port")
	daemon.Flags().BoolVar(&remote, "allow-remote", false, "allow remote binding; configure a TLS reverse proxy")
	root.AddCommand(daemon)
	var dryRun, structured bool
	var expected, id string
	apply := &cobra.Command{Use: "apply FILE", Short: "Atomically apply a manifest to the running daemon", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read manifest")
		}
		request := watchrun.ApplyRequest{Manifest: string(data), DryRun: dryRun}
		if expected != "" {
			if id == "" {
				return Fail(cmd.ErrOrStderr(), structured, "invalid_arguments", "--expected-revision requires --watch")
			}
			request.Expected = map[string]string{id: expected}
		}
		return call(cmd, dir, structured, http.MethodPost, "/v1/apply", request)
	}}
	apply.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without applying them")
	apply.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
	apply.Flags().StringVar(&expected, "expected-revision", "", "reject changes if the current revision differs")
	apply.Flags().StringVar(&id, "watch", "", "watch ID for the expected revision")
	root.AddCommand(apply)
	watchCmd := &cobra.Command{Use: "watch", Short: "Inspect and manage watches"}
	for _, name := range []string{"list", "inspect"} {
		var structured bool
		command := &cobra.Command{Use: name, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			path := "/v1/watches"
			if cmd.Name() == "inspect" {
				path += "/" + url.PathEscape(args[0])
			}
			return call(cmd, dir, structured, http.MethodGet, path, nil)
		}}
		if name == "inspect" {
			command.Use = "inspect ID"
			command.Args = cobra.ExactArgs(1)
		}
		command.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
		watchCmd.AddCommand(command)
	}
	root.AddCommand(watchCmd)
}
func call(cmd *cobra.Command, dir string, structured bool, method, path string, request any) error {
	client, err := control.Connect(dir)
	if err != nil {
		return Fail(cmd.ErrOrStderr(), structured, "daemon_unavailable", err.Error())
	}
	result, err := client.Call(cmd.Context(), method, path, request)
	if err != nil {
		var api *control.APIError
		if errors.As(err, &api) {
			return Fail(cmd.ErrOrStderr(), structured, api.Code, api.Message)
		}
		return Fail(cmd.ErrOrStderr(), structured, "daemon_unavailable", err.Error())
	}
	if structured {
		return Write(cmd.OutOrStdout(), result)
	}
	var value any
	if err := json.Unmarshal(result, &value); err != nil {
		return err
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
