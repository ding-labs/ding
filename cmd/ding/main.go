package main

import (
	"github.com/ding-labs/ding/internal/watchcli"
	"os"
)

var version = "dev"

func main() {
	os.Exit(watchcli.ExitCode(watchcli.Execute(version, os.Args[1:], os.Stdout, os.Stderr)))
}
