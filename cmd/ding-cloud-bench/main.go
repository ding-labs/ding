package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/ding-labs/ding/internal/cloud"
)

func main() {
	var o cloud.BenchmarkOptions
	flag.IntVar(&o.Accounts, "accounts", 100, "synthetic workspace count (1–100)")
	flag.DurationVar(&o.Duration, "duration", 30*time.Second, "sampling period after enrollment (use >5m for repeated observations)")
	flag.DurationVar(&o.Latency, "latency", 100*time.Millisecond, "synthetic response latency")
	flag.IntVar(&o.BodyBytes, "body-bytes", 1024, "synthetic HTTP response bytes")
	flag.BoolVar(&o.SameHost, "same-host", false, "exercise shared per-host throttling instead of distinct synthetic hosts")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := cloud.Benchmark(ctx, o)
	if err == nil {
		err = cloud.WriteBenchmark(os.Stdout, result)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
