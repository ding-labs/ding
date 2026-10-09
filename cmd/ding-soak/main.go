// ding-soak is a local qualification driver, not a shipped Ding command.
// It uses real wall time, real HTTP polls, and the production push/commit path.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type histogram struct {
	Counts [10001]uint64
	Count  uint64
}

func (h *histogram) add(d time.Duration) {
	n := max(0, min(10000, int((d+time.Millisecond-1)/time.Millisecond)))
	h.Counts[n]++
	h.Count++
}
func (h histogram) p95() int {
	var n uint64
	for i, count := range h.Counts {
		n += count
		if n*100 >= h.Count*95 {
			return i
		}
	}
	return 10000
}

type sample struct {
	ElapsedSeconds float64     `json:"elapsedSeconds"`
	RSS            int64       `json:"rssBytes"`
	Disk           int64       `json:"databaseAndWALBytes"`
	Usage          store.Usage `json:"usage"`
}
type metrics struct {
	mu                                 sync.Mutex
	Steady, Burst                      histogram
	Attempts, Accepted, Rejected, Late uint64
	Busy                               uint64
	Errors                             map[string]uint64
}
type counter struct{ n atomic.Uint64 }

func (c *counter) Write(b []byte) (int, error) { c.n.Add(1); return len(b), nil }

type report struct {
	Started          time.Time         `json:"started"`
	Finished         time.Time         `json:"finished,omitempty"`
	RequestedSeconds float64           `json:"requestedSeconds"`
	ElapsedSeconds   float64           `json:"elapsedSeconds"`
	Status           string            `json:"status"`
	Revision         string            `json:"revision"`
	GoVersion        string            `json:"goVersion"`
	CPUs             int               `json:"gomaxprocs"`
	History          string            `json:"history"`
	Warmup           string            `json:"warmup"`
	BurstEvery       string            `json:"burstEvery"`
	HTTPRequests     uint64            `json:"httpRequests"`
	Attempts         uint64            `json:"pushAttempts"`
	Accepted         uint64            `json:"acceptedPushes"`
	Rejected         uint64            `json:"rejectedPushes"`
	Busy             uint64            `json:"retryableBusyAdmissions"`
	Late             uint64            `json:"arrivalsLateOver25ms"`
	Delivered        uint64            `json:"consoleDeliveries"`
	SteadyP95MS      int               `json:"steadyCommitP95MillisecondsUpperBound"`
	BurstP95MS       int               `json:"burstCommitP95MillisecondsUpperBound"`
	PeakRSS          int64             `json:"peakRSSBytes"`
	GrowthPercent    float64           `json:"rssMedianGrowthPercent"`
	Errors           map[string]uint64 `json:"errors"`
	Samples          []sample          `json:"samples"`
	Failures         []string          `json:"failures"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	duration := flag.Duration("duration", 24*time.Hour, "real elapsed run duration")
	out := flag.String("out", "", "new private results directory")
	warmup := flag.Duration("warmup", 15*time.Minute, "RSS baseline begins after this interval")
	history := flag.Duration("history", time.Hour, "ordinary history retention (explicit capacity fixture setting)")
	burstEvery := flag.Duration("burst-every", time.Hour, "30 seconds at 200 pushes/s each interval")
	sampleEvery := flag.Duration("sample-every", time.Minute, "metrics interval")
	revision := flag.String("revision", "unknown", "tested git revision")
	profile := flag.String("cpu-profile", "", "optional CPU profile path")
	flag.Parse()
	if *profile != "" {
		file, err := os.Create(*profile)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := pprof.StartCPUProfile(file); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("RSS measurement requires Linux /proc")
	}
	if *duration < time.Minute || *warmup < 0 || *warmup+2**sampleEvery >= *duration || *burstEvery < time.Minute || *sampleEvery < time.Second || *duration / *sampleEvery > 10000 || *history < time.Second || *out == "" {
		return fmt.Errorf("invalid duration, warmup, history, burst, sample or output setting")
	}
	if err := os.Mkdir(*out, 0700); err != nil {
		return err
	}
	dir := filepath.Join(*out, "state")
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		return err
	}
	defer s.Close()
	a := watchrun.New(s)
	a.Limits.Retention = *history
	deliveries := &counter{}
	a.Output = deliveries
	var polls atomic.Uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"value":0}`)
	}))
	defer server.Close()
	for i := 0; i < 100; i++ {
		manifest := fmt.Sprintf("apiVersion: ding.ing/v1alpha1\nkind: Watch\nmetadata: {id: http-%03d}\nspec:\n  source: {type: http, url: %s, every: 5s, timeout: 2s, fields: {value: value}}\n  condition: {field: value, operator: gte, value: 1}\n", i, server.URL)
		if _, err := a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: manifest}); err != nil {
			return err
		}
	}
	manifest := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: qualification}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: push}
spec:
  source: {type: push, fields: {value: value}}
  condition: {field: value, operator: gte, value: 1}
  policy: {trigger: level, interval: 0s}
  destinations: [{ref: qualification, events: [firing]}]
`
	if _, err := a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		return err
	}
	record, err := a.Record(context.Background(), "push")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	running := make(chan error, 1)
	go func() { running <- a.Run(ctx) }()
	// The capacity phase begins once the 100 HTTP sources have each accepted
	// their first poll. Startup's simultaneous poll burst is a separate overload
	// case; counting it as steady-state capacity would conflate two workloads.
	startupDeadline := time.Now().Add(30 * time.Second)
	for {
		all, err := a.List(ctx)
		if err != nil {
			return err
		}
		ready := 0
		for _, r := range all {
			if r.Plan.Definition.Spec.Source.Type == "http" && !r.LastInputAt.IsZero() {
				ready++
			}
		}
		if ready == 100 {
			break
		}
		if time.Now().After(startupDeadline) {
			return fmt.Errorf("100 HTTP sources did not initialize")
		}
		time.Sleep(20 * time.Millisecond)
	}
	initialPolls := polls.Load()
	started := time.Now()
	deadline := started.Add(*duration)
	m := &metrics{Errors: map[string]uint64{}}
	r := report{Started: started.UTC(), RequestedSeconds: duration.Seconds(), Status: "running", Revision: *revision, GoVersion: runtime.Version(), CPUs: runtime.GOMAXPROCS(0), History: history.String(), Warmup: warmup.String(), BurstEvery: burstEvery.String(), Samples: []sample{}, Failures: []string{}}
	var work sync.WaitGroup
	producer := make(chan struct{}, 256)
	var id uint64
	nextSample := started
	push := func(scheduled time.Time, burst bool) {
		id++
		inputID := strconv.FormatUint(id, 10)
		m.mu.Lock()
		m.Attempts++
		if time.Since(scheduled) > 25*time.Millisecond {
			m.Late++
		}
		m.mu.Unlock()
		select {
		case producer <- struct{}{}:
		default:
			m.mu.Lock()
			m.Rejected++
			m.Errors["producer backlog exceeded 256 requests"]++
			m.mu.Unlock()
			return
		}
		work.Add(1)
		go func() {
			defer work.Done()
			defer func() { <-producer }()
			begin := time.Now()
			for {
				err := a.Reserve()
				if err == nil {
					break
				}
				m.mu.Lock()
				m.Busy++
				m.mu.Unlock()
				if err != watchrun.ErrBusy || time.Since(begin) >= time.Second {
					m.mu.Lock()
					m.Rejected++
					m.Errors[err.Error()]++
					m.mu.Unlock()
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			defer a.Release()
			_, err := a.IngestReserved(ctx, record, []byte(`{"value":2}`), inputID)
			elapsed := time.Since(begin)
			m.mu.Lock()
			defer m.mu.Unlock()
			if err != nil {
				m.Rejected++
				m.Errors[err.Error()]++
				return
			}
			m.Accepted++
			if burst {
				m.Burst.add(elapsed)
			} else {
				m.Steady.add(elapsed)
			}
		}()
	}
	update := func() error {
		r.ElapsedSeconds = time.Since(started).Seconds()
		r.HTTPRequests = polls.Load() - initialPolls
		r.Delivered = deliveries.n.Load()
		m.mu.Lock()
		r.Attempts, r.Accepted, r.Rejected, r.Late = m.Attempts, m.Accepted, m.Rejected, m.Late
		r.Busy = m.Busy
		r.SteadyP95MS, r.BurstP95MS = m.Steady.p95(), m.Burst.p95()
		r.Errors = map[string]uint64{}
		for k, v := range m.Errors {
			r.Errors[k] = v
		}
		m.mu.Unlock()
		var usage store.Usage
		if err := s.View(context.Background(), func(tx *store.Tx) error { var e error; usage, e = tx.Usage(); return e }); err != nil {
			return err
		}
		rss, peak, err := readMemory()
		if err != nil {
			return err
		}
		r.PeakRSS = max(r.PeakRSS, peak)
		var disk int64
		for _, name := range []string{"ding.db", "ding.db-wal", "ding.db-shm"} {
			if info, e := os.Stat(filepath.Join(dir, name)); e == nil {
				disk += info.Size()
			}
		}
		r.Samples = append(r.Samples, sample{r.ElapsedSeconds, rss, disk, usage})
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		temp := filepath.Join(*out, "progress.tmp")
		if err := os.WriteFile(temp, data, 0600); err != nil {
			return err
		}
		return os.Rename(temp, filepath.Join(*out, "progress.json"))
	}
	// Absolute arrival times expose driver lateness; they never silently reduce
	// the target rate to the speed at which ingestion happens to complete.
	for next := started; next.Before(deadline); {
		if delay := time.Until(next); delay > 0 {
			time.Sleep(delay)
		}
		select {
		case err := <-running:
			if err == nil {
				err = fmt.Errorf("runtime stopped early")
			}
			return err
		default:
		}
		elapsed := next.Sub(started)
		burst := elapsed%*burstEvery < 30*time.Second
		push(next, burst)
		step := time.Second
		if burst {
			step = 5 * time.Millisecond
		}
		next = next.Add(step)
		if time.Now().After(nextSample) {
			if err := update(); err != nil {
				return err
			}
			nextSample = time.Now().Add(*sampleEvery)
		}
	}
	if remaining := time.Until(deadline); remaining > 0 {
		time.Sleep(remaining)
	}
	work.Wait()
	cancel()
	if err := <-running; err != nil {
		return err
	}
	r.Finished = time.Now().UTC()
	r.Status = "finished"
	if err := update(); err != nil {
		return err
	}
	if r.ElapsedSeconds < duration.Seconds() {
		r.Failures = append(r.Failures, "requested wall time not completed")
	}
	if r.Rejected > 0 {
		r.Failures = append(r.Failures, "push rejections")
	}
	if r.Late*100 > r.Attempts {
		r.Failures = append(r.Failures, "more than 1% of scheduled arrivals over 25ms late")
	}
	if r.Delivered != r.Accepted {
		r.Failures = append(r.Failures, "one firing/delivery per accepted push not preserved")
	}
	if r.PeakRSS >= 250<<20 {
		r.Failures = append(r.Failures, "peak RSS at or above 250 MiB")
	}
	if r.SteadyP95MS >= 100 {
		r.Failures = append(r.Failures, "steady commit p95 at or above 100ms")
	}
	if r.HTTPRequests < uint64(duration.Seconds()/5*100*.95) {
		r.Failures = append(r.Failures, "fewer than 95% of expected HTTP polls")
	}
	var baseline, final []int64
	window := min(10**sampleEvery, (*duration-*warmup)/3)
	for _, v := range r.Samples {
		at := time.Duration(v.ElapsedSeconds * float64(time.Second))
		if at >= *warmup && at < *warmup+window {
			baseline = append(baseline, v.RSS)
		}
		if at >= *duration-window {
			final = append(final, v.RSS)
		}
	}
	if len(baseline) == 0 || len(final) == 0 {
		r.Failures = append(r.Failures, "insufficient post-warmup memory samples")
	} else {
		r.GrowthPercent = (float64(median(final))/float64(median(baseline)) - 1) * 100
		if r.GrowthPercent >= 10 {
			r.Failures = append(r.Failures, "post-warmup RSS growth at or above 10%")
		}
	}
	if a.Health() != "" {
		r.Failures = append(r.Failures, "runtime ended with an error")
	}
	backup := filepath.Join(*out, "verified-backup.db")
	if err := s.Backup(context.Background(), backup); err != nil {
		return err
	}
	health, err := s.Health(context.Background())
	if err != nil {
		return err
	}
	if health.Integrity != "ok" {
		r.Failures = append(r.Failures, "store integrity")
	}
	if len(r.Failures) == 0 {
		r.Status = "passed"
	} else {
		r.Status = "failed"
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "result.json"), data, 0600); err != nil {
		return err
	}
	fmt.Printf("%s: elapsed %.1fs, accepted %d, rejected %d, p95 steady/burst %d/%dms, peak RSS %.1f MiB, growth %.1f%%\n", r.Status, r.ElapsedSeconds, r.Accepted, r.Rejected, r.SteadyP95MS, r.BurstP95MS, float64(r.PeakRSS)/(1<<20), r.GrowthPercent)
	if len(r.Failures) > 0 {
		return fmt.Errorf("qualification failures: %s", strings.Join(r.Failures, ", "))
	}
	return nil
}
func median(values []int64) int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[len(values)/2]
}
func readMemory() (int64, int64, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, err
	}
	return parseMemory(string(data))
}
func parseMemory(data string) (int64, int64, error) {
	var rss, peak int64
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && (fields[0] == "VmRSS:" || fields[0] == "VmHWM:") && fields[2] == "kB" {
			n, e := strconv.ParseInt(fields[1], 10, 64)
			if e != nil || n <= 0 {
				return 0, 0, fmt.Errorf("invalid process memory")
			}
			if fields[0] == "VmRSS:" {
				rss = n * 1024
			} else {
				peak = n * 1024
			}
		}
	}
	if rss == 0 || peak < rss {
		return 0, 0, fmt.Errorf("VmRSS/VmHWM unavailable")
	}
	return rss, peak, nil
}
