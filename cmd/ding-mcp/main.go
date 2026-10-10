package main

import (
	"fmt"
	"github.com/ding-labs/ding/internal/mcpcli"
	"os"
)

var version = "dev"

func main() {
	cmd := mcpcli.Command(mcpcli.Version(version))
	cmd.Use = "ding-mcp"
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ding-mcp:", err)
		os.Exit(1)
	}
}
