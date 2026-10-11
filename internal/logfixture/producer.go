// Package logfixture provides an independent, paced subprocess workload. It is
// used by qualification tools, never by the application collector.
package logfixture

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type Result struct {
	Records            int64       `json:"records"`
	Warnings           int64       `json:"warnings"`
	Bytes              int64       `json:"bytes"`
	LateWrites         int64       `json:"lateWrites"`
	Started            time.Time   `json:"started"`
	Finished           time.Time   `json:"finished"`
	WarningTransitions []time.Time `json:"warningTransitions"`
}

// Produce uses a fixed offered schedule, alternating five-second warning and
// info phases. Writes go directly to the supplied outputs without buffering.
func Produce(out, errout io.Writer, rate, size int, duration time.Duration, format string) (Result, error) {
	r := Result{Started: time.Now(), WarningTransitions: []time.Time{}}
	if rate < 1 || rate > 1000000 || size < 64 || size > 64<<10 || duration <= 0 || duration > 48*time.Hour || format != "text" && format != "jsonl" {
		return r, fmt.Errorf("invalid log workload")
	}
	line := func(level string) []byte {
		if format == "jsonl" {
			b, _ := json.Marshal(map[string]string{"level": level, "message": ""})
			b, _ = json.Marshal(map[string]string{"level": level, "message": strings.Repeat("x", size-len(b)-1)})
			return append(b, '\n')
		}
		return []byte(level + " " + strings.Repeat("x", size-len(level)-2) + "\n")
	}
	warning, info := line("WARN"), line("INFO")
	total := int64(duration.Seconds() * float64(rate))
	for i := int64(0); i < total; i++ {
		due := r.Started.Add(time.Duration(i/int64(rate))*time.Second + time.Duration(i%int64(rate))*time.Second/time.Duration(rate))
		if delay := time.Until(due); delay > 0 {
			time.Sleep(delay)
		}
		if time.Since(due) > 100*time.Millisecond {
			r.LateWrites++
		}
		isWarning := (i/int64(rate)/5)%2 == 0
		data := info
		if isWarning {
			data = warning
			r.Warnings++
			if i%int64(rate*10) == 0 {
				r.WarningTransitions = append(r.WarningTransitions, time.Now())
			}
		}
		writer := out
		if i%2 == 1 {
			writer = errout
		}
		n, err := writer.Write(data)
		r.Bytes += int64(n)
		if err != nil {
			return r, err
		}
		if n != len(data) {
			return r, io.ErrShortWrite
		}
		r.Records++
	}
	r.Finished = time.Now()
	return r, nil
}
