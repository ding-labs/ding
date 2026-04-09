package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version",
		Example: "  ding version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("ding version", version)
		},
	}
}
