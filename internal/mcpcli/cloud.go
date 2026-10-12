package mcpcli

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/ding-labs/ding/internal/mcpremote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func cloudCommand() *cobra.Command {
	group := &cobra.Command{Use: "cloud", Short: "Authorize an explicit workspace-scoped cloud MCP profile"}
	var path string
	var p mcpremote.Profile
	var port int
	connect := &cobra.Command{Use: "connect", Short: "Sign in with OAuth and save a separate private cloud profile", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return safe(mcpremote.Login(ctx, path, p, port, cmd.ErrOrStderr()))
	}}
	connect.Flags().StringVar(&path, "profile", "", "Private cloud profile path (separate from local pairing)")
	connect.Flags().StringVar(&p.URL, "url", "https://cloud.ding.ing/mcp", "Hosted MCP endpoint")
	connect.Flags().StringVar(&p.Issuer, "issuer", "", "Exact registered OAuth issuer")
	connect.Flags().StringVar(&p.ClientID, "client-id", "", "Pre-registered public OAuth client ID")
	connect.Flags().StringVar(&p.Workspace, "workspace", "", "Workspace ID approved for this client in Ding Console")
	connect.Flags().StringSliceVar(&p.Scopes, "scopes", []string{"ding:inspect", "ding:preview"}, "Explicit OAuth scopes; writes require ding:manage or ding:retry")
	connect.Flags().IntVar(&port, "callback-port", 7678, "Registered loopback callback port; path is /callback")
	for _, flag := range []string{"profile", "issuer", "client-id", "workspace"} {
		_ = connect.MarkFlagRequired(flag)
	}
	group.AddCommand(connect)
	var doctorPath string
	doctor := &cobra.Command{Use: "doctor", Short: "Verify the saved OAuth profile and current workspace consent", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return safe(mcpremote.WithSession(cmd.Context(), doctorPath, func(cs *mcp.ClientSession, p mcpremote.Profile) error {
			view, err := mcpremote.Capabilities(cmd.Context(), cs, p)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(view)
		}))
	}}
	doctor.Flags().StringVar(&doctorPath, "profile", "", "Private cloud profile path")
	_ = doctor.MarkFlagRequired("profile")
	group.AddCommand(doctor)
	return group
}
