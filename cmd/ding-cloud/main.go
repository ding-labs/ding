package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

var version = "dev"

func main() {
	config := flag.String("config", "", "owner-only cloud configuration JSON")
	check := flag.Bool("check-config", false, "validate configuration shape without starting services")
	flag.Parse()
	var c cloud.Config
	err := mcpconfig.ReadPrivateJSON(*config, &c, true)
	if err == nil {
		err = c.Validate()
	}
	if err == nil && !*check {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		err = cloud.Run(ctx, c, version, os.Stderr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		fmt.Println("Cloud configuration shape is valid. Identity, key access, signing, live-provider and deployment qualification remain separate checks.")
	}
}
