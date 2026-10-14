package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/ding-labs/ding/internal/cloud"
)

func main() {
	var o cloud.BenchmarkOptions
	flag.IntVar(&o.Workspaces, "workspaces", 100, "synthetic workspace count (1–100)")
	flag.DurationVar(&o.Duration, "duration", 30*time.Second, "sampling period after enrollment (use >5m for repeated observations)")
	flag.DurationVar(&o.Latency, "latency", 100*time.Millisecond, "synthetic response latency")
	flag.IntVar(&o.BodyBytes, "body-bytes", 1024, "synthetic HTTP response bytes")
	flag.BoolVar(&o.SameHost, "same-host", false, "exercise shared per-host throttling instead of distinct synthetic hosts")
	pushFile := flag.String("push-producers", "", "JSON producer file for real HTTPS ingestion qualification")
	ledger := flag.String("ledger", "", "new private JSONL request ledger")
	rate := flag.Int("rate", 10, "offered push requests per second")
	workers := flag.Int("push-workers", 16, "push generator workers (1–1024)")
	noRetry := flag.Bool("push-no-retry", false, "measure first attempts without retry backoff")
	ledgerBatch := flag.Int("push-ledger-batch", 1, "entries per durable ledger sync (1–1024)")
	compact := flag.Bool("push-compact-ledger", false, "store producer bodies once in the ledger")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *pushFile != "" {
		raw, err := os.ReadFile(*pushFile)
		var producers []cloud.PushProducer
		if err == nil {
			err = json.Unmarshal(raw, &producers)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		result, err := cloud.BenchmarkPush(ctx, cloud.PushBenchmarkOptions{Producers: producers, Duration: o.Duration, Rate: *rate, Ledger: *ledger, Workers: *workers, NoRetry: *noRetry, LedgerBatch: *ledgerBatch, CompactLedger: *compact})
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(result)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	result, err := cloud.Benchmark(ctx, o)
	if err == nil {
		err = cloud.WriteBenchmark(os.Stdout, result)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
