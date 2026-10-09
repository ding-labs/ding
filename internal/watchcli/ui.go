package watchcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ding-labs/ding/internal/control"
	"github.com/spf13/cobra"
)

func uiCommand(dir *string) *cobra.Command {
	var noOpen bool
	cmd := &cobra.Command{Use: "ui", Short: "Open Ding Console with a single-use browser session link", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		raw, err := request(cmd, *dir, false, "POST", "/v1/browser/handoff", struct{}{})
		if err != nil {
			return err
		}
		var h control.BrowserHandoff
		if err := json.Unmarshal(raw, &h); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Ding Console (link expires in 60 seconds):\n%s\n", h.URL)
		if noOpen {
			return nil
		}
		var opener *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			opener = exec.Command("open", h.URL)
		case "windows":
			opener = exec.Command("rundll32", "url.dll,FileProtocolHandler", h.URL)
		default:
			opener = exec.Command("xdg-open", h.URL)
		}
		if err := opener.Start(); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Open the printed link in your browser.")
			return nil
		}
		go func() { _ = opener.Wait() }()
		return nil
	}}
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print a launch link without opening a browser")
	return cmd
}
func consoleReference(topic string) (string, error) {
	root := Root("reference")
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var out bytes.Buffer
	if strings.HasPrefix(topic, "completion:") {
		switch strings.TrimPrefix(topic, "completion:") {
		case "bash":
			err := root.GenBashCompletion(&out)
			return out.String(), err
		case "zsh":
			err := root.GenZshCompletion(&out)
			return out.String(), err
		case "fish":
			err := root.GenFishCompletion(&out, true)
			return out.String(), err
		case "powershell":
			err := root.GenPowerShellCompletion(&out)
			return out.String(), err
		default:
			return "", fmt.Errorf("unknown shell")
		}
	}
	args := strings.Fields(topic)
	c, remaining, err := root.Find(args)
	if err != nil || len(remaining) > 0 {
		return "", fmt.Errorf("unknown command")
	}
	c.SetOut(&out)
	c.SetErr(&out)
	if err := c.Help(); err != nil {
		return "", err
	}
	return out.String(), nil
}
