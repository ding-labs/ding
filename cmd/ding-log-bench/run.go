package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/logfixture"
	"github.com/ding-labs/ding/internal/logs"
)

type Report struct {
	OS, Architecture, Go, Transport, Format string
	Rate, RecordBytes                       int
	Producer                                logfixture.Result
	Collector                               logs.Statistics
	Runtime                                 logfixture.RuntimeMetrics
	ForwardedBytes                          int64
	MaxHeapBytes                            uint64
	FiringLatencyMS                         []float64
	DeliveryLatencyMS                       []float64
	FiringP95MS, DeliveryP95MS              float64
	Passed                                  bool
}

type byteCounter struct{ n atomic.Int64 }

func (c *byteCounter) Write(b []byte) (int, error) { c.n.Add(int64(len(b))); return len(b), nil }

func run(parent context.Context, o options) (report Report, err error) {
	report = Report{OS: runtime.GOOS, Architecture: runtime.GOARCH, Go: runtime.Version(), Transport: o.transport, Format: o.format, Rate: o.rate, RecordBytes: o.size}
	ctx, cancel := context.WithTimeout(parent, o.duration+20*time.Second)
	defer cancel()
	r, err := logfixture.NewRuntime(ctx, filepath.Join(o.out, "state"), o.format)
	if err != nil {
		return report, err
	}
	defer r.Close()
	c, err := logs.NewCollector(logs.ParserConfig{Format: o.format}, logs.WindowConfig{Window: time.Second, MinEvents: 1})
	if err != nil {
		return report, err
	}
	defer c.Close()
	output := &byteCounter{}
	done := make(chan error, 1)
	go func() {
		done <- logfixture.Capture(ctx, logfixture.CaptureOptions{Duration: o.duration, Rate: o.rate, Size: o.size, Format: o.format, Transport: o.transport, Out: o.out}, c, output)
	}()
	// Cancellation waits for transport writers before closing the analysis queue.
	finished := false
	defer func() {
		cancel()
		if !finished {
			<-done
		}
	}()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var end time.Time
	for end.IsZero() || time.Now().Before(end) {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		case err := <-done:
			finished, end, done = true, time.Now().Add(2*time.Second), nil
			if err != nil {
				return report, err
			}
		case <-tick.C:
			if err := r.Publish(ctx, c.Snapshot(), false); err != nil {
				return report, err
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			report.MaxHeapBytes = max(report.MaxHeapBytes, m.HeapAlloc)
		}
	}
	if err := r.Publish(ctx, c.Snapshot(), true); err != nil {
		return report, err
	}
	report.Collector, report.ForwardedBytes = c.Statistics(), output.n.Load()
	data, err := os.ReadFile(filepath.Join(o.out, "producer.json"))
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(data, &report.Producer); err != nil {
		return report, err
	}
	report.Runtime, err = r.Metrics(ctx)
	if err != nil {
		return report, err
	}
	// Skip the first firing: it includes startup warmup. Every later warning
	// transition is independently timestamped by the producer before its write.
	latency := func(observed []time.Time) []float64 {
		values := []float64{}
		for i := 1; i < len(report.Producer.WarningTransitions) && i < len(observed); i++ {
			values = append(values, float64(observed[i].Sub(report.Producer.WarningTransitions[i]))/float64(time.Millisecond))
		}
		return values
	}
	report.FiringLatencyMS = latency(report.Runtime.FiringCommitted)
	report.DeliveryLatencyMS = latency(report.Runtime.FiringDelivered)
	report.FiringP95MS, report.DeliveryP95MS = p95(report.FiringLatencyMS), p95(report.DeliveryLatencyMS)
	report.Passed = report.Producer.Records == int64(o.duration.Seconds()*float64(o.rate)) && report.Collector.Records == report.Producer.Records && report.Collector.Warnings == report.Producer.Warnings && report.Collector.Invalid == 0 && report.Collector.Dropped == 0 && report.ForwardedBytes == report.Producer.Bytes && report.Runtime.PollingHealthy && report.Runtime.Events["firing"] > 0 && report.Runtime.Events["recovered"] > 0 && report.Runtime.Events["source_error"] == 0 && len(report.FiringLatencyMS) > 0 && len(report.DeliveryLatencyMS) == len(report.FiringLatencyMS) && report.FiringP95MS <= 2000 && report.DeliveryP95MS <= 3000 && report.Producer.LateWrites == 0
	if !report.Passed {
		return report, fmt.Errorf("log qualification gate failed; inspect report.json")
	}
	return report, nil
}

func p95(values []float64) float64 {
	if len(values) == 0 {
		return -1
	}
	copy := append([]float64(nil), values...)
	sort.Float64s(copy)
	return copy[(len(copy)*95+99)/100-1]
}
