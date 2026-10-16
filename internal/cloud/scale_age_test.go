package cloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Seed through the same authenticated HTTPS path. Preload counts are reported
// separately and form the baseline; no invented observations or receipts.
func scalePreload(t *testing.T, lab *scaleLab, c scaleConfig, out string) PushBenchmarkResult {
	t.Helper()
	if c.Preload == 0 {
		return PushBenchmarkResult{}
	}
	if c.Groups > c.Outputs && c.Preload >= c.Groups/c.Outputs {
		seed := c
		seed.Outputs = min(100, c.Groups)
		seed.BodyBytes = 0
		bodies, err := seed.bodies()
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		count := 0
		for i := range lab.tenants {
			for _, watchID := range c.watchIDs() {
				p := PushProducer{URL: lab.http.URL + "/v1/ingest/" + watchID, TokenEnv: fmt.Sprintf("DING_LOCAL_SCALE_%d", i)}
				for j, body := range bodies {
					status, _ := scaleSend(t, lab, p, fmt.Sprintf("seed-groups-%d-%s-%d", i, watchID, j), body)
					if status != 202 {
						t.Fatal("group seed rejected", status)
					}
					count++
				}
			}
		}
		t.Logf("seeded %d groups per workspace using %d real batches", c.Groups, count)
		return PushBenchmarkResult{Offered: count, Accepted: count, FirstAccepted: count, Attempts: count, DurationSeconds: time.Since(start).Seconds()}
	}
	rate := min(500, c.Preload)
	seconds := (c.Preload + rate - 1) / rate
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Rate: rate, Duration: time.Duration(seconds) * time.Second, Workers: c.Workers, Ledger: filepath.Join(out, "preload.jsonl"), LedgerBatch: 32, CompactLedger: true, NoRetry: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preload actual=%d target=%d", r.Accepted, c.Preload)
	accepted := r.Accepted
	// A rejected preload is not an aged-store fixture. Fill at a lower rate,
	// retain every round's ledger/result, and bound the extra work to 64 rounds.
	// Start below observed accepted throughput, then approach a useful fill
	// rate. A fixed 100/s can repeatedly reject on a constrained backend.
	fillRate := max(1, min(100, int(float64(r.Accepted)/max(.001, r.DurationSeconds)*.8)))
	for round := 0; accepted < c.Preload && round < 64; round++ {
		for _, path := range []string{lab.root, out} {
			if free, err := freeDiskBytes(path); err != nil || free < 20<<30 {
				t.Fatal("preload free-disk floor reached")
			}
		}
		fillSeconds := min(60, (c.Preload-accepted+fillRate-1)/fillRate)
		fill, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Rate: fillRate, Duration: time.Duration(fillSeconds) * time.Second, Workers: c.Workers, Ledger: filepath.Join(out, fmt.Sprintf("preload-fill-%d.jsonl", round+1)), LedgerBatch: 32, CompactLedger: true, NoRetry: true})
		if err != nil {
			t.Fatal(err)
		}
		lab.preloadFills = append(lab.preloadFills, fill)
		accepted += fill.Accepted
		if fill.Accepted != fill.Offered || fill.GeneratorBusy != 0 || fill.Unconfirmed != 0 {
			fillRate = max(1, min(fillRate, int(float64(fill.Accepted)/max(.001, fill.DurationSeconds)*.8)))
		} else if fill.P99MS < 250 {
			fillRate = min(100, fillRate+max(1, fillRate/4))
		}
	}
	if accepted < c.Preload {
		t.Fatalf("preload incomplete after bounded fill: actual=%d target=%d", accepted, c.Preload)
	}
	t.Logf("preload complete actual=%d target=%d fillRounds=%d", accepted, c.Preload, len(lab.preloadFills))
	return r
}

func scaleReads(ctx context.Context, lab *scaleLab) func() (int, float64, int) {
	done := make(chan struct{})
	count, errors := 0, 0
	hist := make([]int, 120001)
	go func() {
		defer close(done)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				start := time.Now()
				index := i % len(lab.tenants)
				i++
				readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				req, err := http.NewRequestWithContext(readCtx, "GET", lab.http.URL+"/v1/watches/ci", nil)
				if err == nil {
					req.Header.Set("Authorization", "Bearer "+os.Getenv(fmt.Sprintf("DING_LOCAL_SCALE_%d", index)))
					var response *http.Response
					response, err = lab.http.Client().Do(req)
					if err == nil {
						_, err = io.Copy(io.Discard, response.Body)
						response.Body.Close()
						if err == nil && response.StatusCode != 200 {
							err = fmt.Errorf("read status %d", response.StatusCode)
						}
					}
				}
				cancel()
				if err != nil && ctx.Err() == nil {
					errors++
				}
				count++
				hist[min(int(time.Since(start).Milliseconds()), len(hist)-1)]++
			}
		}
	}()
	return func() (int, float64, int) { <-done; return count, histogramPercentile(hist, 99), errors }
}

func TestScalePreloadIsRealAcceptedHistory(t *testing.T) {
	c := scaleConfig{Name: "aged", Rate: 10, Seconds: 1, Preload: 10}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	r := scalePreload(t, lab, c, t.TempDir())
	a := scaleAccounts(t, lab)
	if r.Accepted != 10 || a[0].Usage.Requests != 10 || a[0].Rows.Observations != 10 {
		t.Fatal(fmt.Sprint(r, a))
	}
}

func TestScaleGroupSeedPopulatesEveryConfiguredGroup(t *testing.T) {
	c := scaleConfig{Name: "groups", Rate: 10, Seconds: 1, Groups: 1000, Preload: 1000}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	r := scalePreload(t, lab, c, t.TempDir())
	a := scaleAccounts(t, lab)
	if r.Accepted != 10 || a[0].Rows.Entities != 1000 || a[0].Rows.Observations != 1000 {
		t.Fatal(r, a)
	}
}

func TestScalePreloadFillsRejectedAttempts(t *testing.T) {
	c := scaleConfig{Name: "preload-fill", Rate: 10, Seconds: 1, Preload: 40, Defaults: true}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	out := t.TempDir()
	r := scalePreload(t, lab, c, out)
	accepted := r.Accepted
	for i, fill := range lab.preloadFills {
		accepted += fill.Accepted
		if _, err := os.Stat(filepath.Join(out, fmt.Sprintf("preload-fill-%d.jsonl", i+1))); err != nil {
			t.Fatal(err)
		}
	}
	actual := scaleAccounts(t, lab)[0].Usage.Requests
	if len(lab.preloadFills) == 0 || accepted < c.Preload || actual != int64(accepted) {
		t.Fatal(r, lab.preloadFills, actual)
	}
}
