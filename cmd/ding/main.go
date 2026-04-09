package main

import (
	"os"

	"github.com/zuchka/ding/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.BuildRoot(version).Execute(); err != nil {
		os.Exit(1)
	}
}
