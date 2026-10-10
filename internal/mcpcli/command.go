// Package mcpcli exposes the same command tree through ding mcp and ding-mcp.
package mcpcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/ding-labs/ding/internal/mcpsetup"
	"github.com/ding-labs/ding/internal/mcpui"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func Command(version string) *cobra.Command {
	root := &cobra.Command{Use: "mcp", Short: "Connect Ding to LLM clients through MCP", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.SetVersionTemplate("{{.Version}}\n")
	var config, state, name string
	var days int
	var manage, retry bool
	var refs, revisions []string
	pair := &cobra.Command{Use: "pair", Short: "Create a scoped grant using local administrator access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return safe(mcpsetup.Pair(cmd.Context(), mcpsetup.PairOptions{State: state, Config: config, Name: name, Days: days, Manage: manage, Retry: retry, SecretRefs: refs, CommandRevisions: revisions}, cmd.OutOrStdout()))
	}}
	pair.Flags().StringVar(&state, "state", "", "Ding daemon state directory")
	_ = pair.MarkFlagRequired("state")
	pair.Flags().StringVar(&config, "config", mcpconfig.DefaultPath(), "Private connection file")
	pair.Flags().StringVar(&name, "name", "Ding MCP", "Grant name")
	pair.Flags().IntVar(&days, "days", 90, "Grant lifetime, 1–365 days")
	pair.Flags().BoolVar(&manage, "manage", false, "Allow applying and changing watches")
	pair.Flags().BoolVar(&retry, "retry", false, "Allow retrying failed notifications")
	pair.Flags().StringArrayVar(&refs, "allow-secret-ref", nil, "Explicitly allow an environment variable reference, never its value")
	pair.Flags().StringArrayVar(&revisions, "allow-command-revision", nil, "Explicitly allow this exact compiled command watch revision")
	root.AddCommand(pair)
	var setupState, setupConfig string
	setup := &cobra.Command{Use: "setup", Short: "Open the guided local pairing window", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return safe(mcpsetup.Run(ctx, setupState, setupConfig, cmd.ErrOrStderr()))
	}}
	setup.Flags().StringVar(&setupState, "state", mcpconfig.DefaultState(), "Ding daemon state directory")
	setup.Flags().StringVar(&setupConfig, "config", mcpconfig.DefaultPath(), "Private connection file")
	root.AddCommand(setup)
	var doctorConfig string
	doctor := &cobra.Command{Use: "doctor", Short: "Inspect the paired connection and granted capabilities", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := mcpconfig.LoadConnection(doctorConfig)
		if err != nil {
			return safe(err)
		}
		client := mcpclient.New(c)
		defer client.Close()
		data, err := client.Call(cmd.Context(), http.MethodGet, "/capabilities", nil, nil)
		if err != nil {
			return safe(err)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(data)
	}}
	doctor.Flags().StringVar(&doctorConfig, "config", mcpconfig.DefaultPath(), "Private connection file")
	root.AddCommand(doctor)
	var grantsState, revoke string
	grants := &cobra.Command{Use: "grants", Short: "List local grants or revoke one", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		method := http.MethodGet
		if revoke != "" {
			method = http.MethodDelete
		}
		_, data, err := mcpsetup.AdminCall(cmd.Context(), grantsState, method, revoke, nil)
		if err != nil {
			return safe(err)
		}
		if revoke != "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Grant revoked.")
			return nil
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}}
	grants.Flags().StringVar(&grantsState, "state", "", "Ding daemon state directory")
	_ = grants.MarkFlagRequired("state")
	grants.Flags().StringVar(&revoke, "revoke", "", "Grant ID to revoke")
	root.AddCommand(grants)
	var serveConfig, httpConfig, transport, host string
	var port int
	serve := &cobra.Command{Use: "serve", Short: "Serve the paired MCP integration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if transport == "http" {
			return safe(serveHTTP(ctx, version, httpConfig, host, port))
		}
		if transport != "stdio" || httpConfig != "" {
			return safe(mcpconfig.ErrConfig)
		}
		c, err := mcpconfig.LoadConnection(serveConfig)
		if err != nil {
			return safe(err)
		}
		client := mcpclient.New(c)
		defer client.Close()
		s, err := mcpserver.New(mcpserver.Options{Version: version, HTML: mcpui.WorkspaceHTML, Resolve: func(context.Context, string, *auth.TokenInfo) (*mcpclient.Client, error) { return client, nil }})
		if err != nil {
			return safe(err)
		}
		input, ok := cmd.InOrStdin().(io.ReadCloser)
		if !ok {
			input = io.NopCloser(cmd.InOrStdin())
		}
		return safe(s.Run(ctx, &mcp.IOTransport{Reader: input, Writer: nopCloser{cmd.OutOrStdout()}}))
	}}
	serve.Flags().StringVar(&serveConfig, "config", mcpconfig.DefaultPath(), "Private connection file")
	serve.Flags().StringVar(&transport, "transport", "stdio", "stdio or http")
	serve.Flags().StringVar(&httpConfig, "http-config", "", "Private self-hosted OAuth configuration")
	serve.Flags().StringVar(&host, "host", "127.0.0.1", "HTTP bind address")
	serve.Flags().IntVar(&port, "port", 7677, "HTTP port")
	root.AddCommand(serve)
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return fmt.Errorf("invalid command options; see --help") })
	return root
}

func safe(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return fmt.Errorf("%s", mcpclient.SafeMessage(err))
}

// Version trims accidental surrounding whitespace from release metadata.
func Version(value string) string { return strings.TrimSpace(value) }
