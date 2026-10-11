// ding-log-bench qualifies live collection through the authenticated HTTP API,
// SQLite evaluator and local console delivery. It never contacts destinations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ding-labs/ding/internal/logfixture"
)

type options struct {
	duration                         time.Duration
	rate, size                       int
	format, transport, out, producer string
}

func main() {
	o := options{}
	flag.DurationVar(&o.duration, "duration", time.Minute, "producer duration; at least 15s for the alert/recovery cycle")
	flag.IntVar(&o.rate, "rate", 1000, "offered records per second")
	flag.IntVar(&o.size, "bytes", 1024, "bytes per record including newline")
	flag.StringVar(&o.format, "format", "text", "text or jsonl")
	flag.StringVar(&o.transport, "transport", "exec", "exec, pipe, or file")
	flag.StringVar(&o.out, "out", "", "new results directory, containing private runtime state")
	flag.StringVar(&o.producer, "producer-report", "", "internal: run the independent producer and write its report")
	flag.Parse()
	var err error
	if o.producer != "" {
		var r logfixture.Result
		r, err = logfixture.Produce(os.Stdout, os.Stderr, o.rate, o.size, o.duration, o.format)
		if err == nil {
			err = save(o.producer, r)
		}
	} else {
		if o.out == "" || o.duration < 15*time.Second || o.duration > 48*time.Hour || o.rate < 1 || o.rate > 1000000 || o.size < 64 || o.size > 64<<10 || o.format != "text" && o.format != "jsonl" || o.transport != "exec" && o.transport != "pipe" && o.transport != "file" {
			err = fmt.Errorf("provide a new --out directory, duration 15s–48h, rate 1–1000000, bytes 64–65536, format text/jsonl, and transport exec/pipe/file")
		} else if err = os.Mkdir(o.out, 0700); err == nil {
			var report Report
			report, err = run(context.Background(), o)
			if writeErr := save(filepath.Join(o.out, "report.json"), report); writeErr != nil {
				err = writeErr
			}
			if err == nil {
				fmt.Println(filepath.Join(o.out, "report.json"))
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func save(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
