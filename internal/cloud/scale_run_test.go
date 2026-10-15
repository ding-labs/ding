package cloud

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

type scaleSample struct {
	At              time.Time
	Heap, Allocated uint64
	Metrics         string
}
type scaleReport struct {
	Config                             scaleConfig
	FixtureDatabase                    string
	StartedAt, EndedAt                 time.Time
	Runtime                            string
	GOMAXPROCS                         int
	Before, After                      []scaleAccounting
	Load                               PushBenchmarkResult
	Resolved                           int
	InternalObservations               int64
	Errors                             []string
	Samples                            []scaleSample
	Preload                            PushBenchmarkResult
	PreloadFills                       []PushBenchmarkResult
	ReadCount, ReadErrors              int
	ReadP99MS                          float64
	ReceiverAttempts, ReceiverAccepted int
	ReceiverUnique                     int
	DeliveryDrainSeconds               float64
	Aborted                            string
	RestartVerified                    bool
	PostChecksVerified                 bool
}

// Opt-in only. The config and evidence directory must be supplied explicitly;
// no production URL, account, state directory, or credential is accepted.
func TestLocalPushScale(t *testing.T) {
	path := os.Getenv("DING_SCALE_CONFIG")
	if path == "" {
		t.Skip("opt-in local stress lab")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c scaleConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("DING_SCALE_OUTPUT")
	if out == "" {
		t.Fatal("DING_SCALE_OUTPUT required")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	report := scaleReport{Config: c, Runtime: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, GOMAXPROCS: runtime.GOMAXPROCS(0)}
	lab := newScaleLab(t, c)
	report.FixtureDatabase = lab.fixtureDatabase
	if c.Noisy > 0 {
		first := lab.producers[0]
		n := (len(lab.producers) - 1) * c.Noisy / (100 - c.Noisy)
		for range n {
			lab.producers = append(lab.producers, first)
		}
	}
	report.Preload = scalePreload(t, lab, c, out)
	report.PreloadFills = lab.preloadFills
	var waitStart func()
	var waitFinish func(context.Context) PushBenchmarkResult
	if os.Getenv("DING_SCALE_EXTERNAL") != "" {
		waitStart, waitFinish = scaleExternal(t, lab, out)
		waitStart()
	}
	report.Before = scaleAccounts(t, lab)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	var finishReads func() (int, float64, int)
	if c.Reads {
		finishReads = scaleReads(ctx, lab)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				for _, path := range []string{lab.root, out} {
					if free, err := freeDiskBytes(path); err != nil || free < 20<<30 {
						report.Aborted = "local free-disk floor reached"
						cancel()
						return
					}
				}
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				w := httptest.NewRecorder()
				lab.server.Pool.Metrics(w, httptest.NewRequest("GET", "http://127.0.0.1/metrics", nil))
				report.Samples = append(report.Samples, scaleSample{At: time.Now().UTC(), Heap: mem.HeapAlloc, Allocated: mem.TotalAlloc, Metrics: w.Body.String()})
			}
		}
	}()
	stopProfiles := func() {}
	if c.Profiles {
		stopProfiles = scaleProfiles(t, out)
	}
	report.StartedAt = time.Now().UTC()
	if lab.receiver != nil && c.Receiver == "outage" {
		lab.receiver.mu.Lock()
		lab.receiver.recoverAt = report.StartedAt.Add(time.Duration(c.Seconds/2) * time.Second)
		lab.receiver.mu.Unlock()
	}
	if waitFinish != nil {
		report.Load = waitFinish(ctx)
	} else {
		report.Load, err = BenchmarkPush(ctx, PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Rate: c.Rate, Duration: time.Duration(c.Seconds) * time.Second, Workers: c.Workers, Ledger: filepath.Join(out, "requests.jsonl"), LedgerBatch: 32, CompactLedger: true, NoRetry: true})
	}
	report.EndedAt = time.Now().UTC()
	stopProfiles()
	cancel()
	wg.Wait()
	if finishReads != nil {
		report.ReadCount, report.ReadP99MS, report.ReadErrors = finishReads()
	}
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	if report.Aborted != "" {
		report.Errors = append(report.Errors, report.Aborted)
	}
	if !c.Noop {
		var errs []string
		report.Resolved, errs = scaleReconcile(t, lab, c, filepath.Join(out, "requests.jsonl"), report.Before)
		report.Errors = append(report.Errors, errs...)
	}
	if c.Restart {
		scaleRestart(t, lab, c)
		_, errs := scaleReconcile(t, lab, c, filepath.Join(out, "requests.jsonl"), report.Before)
		report.Errors = append(report.Errors, errs...)
		report.RestartVerified = len(errs) == 0
	}
	if !c.Noop {
		errs := scaleChecks(t, lab, c, filepath.Join(out, "requests.jsonl"))
		report.Errors = append(report.Errors, errs...)
		report.PostChecksVerified = len(errs) == 0 && report.Load.Accepted > 0
	}
	report.InternalObservations = lab.internalObservations
	report.After = scaleAccounts(t, lab)
	if lab.receiver != nil {
		start := time.Now()
		deadline := start.Add(time.Minute)
		for time.Now().Before(deadline) {
			pending := 0
			for _, a := range report.After {
				pending += a.Rows.Pending
			}
			if pending == 0 {
				break
			}
			time.Sleep(time.Second)
			report.InternalObservations = lab.internalObservations
			report.After = scaleAccounts(t, lab)
		}
		report.DeliveryDrainSeconds = time.Since(start).Seconds()
		lab.receiver.mu.Lock()
		report.ReceiverAttempts, report.ReceiverAccepted = lab.receiver.attempts, lab.receiver.accepted
		report.ReceiverUnique = len(lab.receiver.ids)
		for _, tenant := range lab.tenants {
			inspection, err := tenant.App.Inspect(context.Background(), "ci")
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range inspection.Deliveries {
				if d.Status == "delivered" && !lab.receiver.ids[d.EventID] {
					report.Errors = append(report.Errors, "delivered event absent from receiver ledger")
				}
			}
		}
		data, err := os.ReadFile(filepath.Join(lab.root, "receiver.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := mcpconfig.CreatePrivate(filepath.Join(out, "receiver.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		lab.receiver.mu.Unlock()
	}
	f, err := mcpconfig.CreatePrivate(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewEncoder(f).Encode(report)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("local scale %s: %+v; resolved=%d errors=%v", c.Name, report.Load, report.Resolved, report.Errors)
	if len(report.Errors) > 0 {
		t.Fatal(report.Errors)
	}
}
