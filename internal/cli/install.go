package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <dst>",
		Short: "Copy the DING binary to <dst>",
		Long: `Copy the running DING binary to <dst>.

Used by Kubernetes initContainers and similar deploy patterns where the DING
image ships from scratch (no shell, no cp) and the binary needs to be staged
into a shared volume before another container can run it.

Overwrites <dst> if it already exists. Sets executable mode (0755). Prints
nothing on success; exits non-zero with a descriptive error on failure.`,
		Example: `  # As a Kubernetes initContainer command in a pod spec:
  #   command: ["/ding", "install", "/shared/ding"]
  ding install /usr/local/bin/ding`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra's default behavior on RunE error is to print usage; suppress
			// that here because install errors are about IO, not bad usage.
			cmd.SilenceUsage = true
			return install(args[0])
		},
	}
}

// install copies the running binary to dst with executable mode.
func install(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open self %q: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open destination %q: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q -> %q: %w", src, dst, err)
	}
	return nil
}
