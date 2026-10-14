package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
	rate := min(500, c.Preload)
	seconds := (c.Preload + rate - 1) / rate
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Rate: rate, Duration: time.Duration(seconds) * time.Second, Workers: c.Workers, Ledger: filepath.Join(out, "preload.jsonl"), LedgerBatch: 32, CompactLedger: true, NoRetry: true})
	if err != nil {
		t.Fatal(err)
	}
	// Complete group coverage deterministically after the timed seed attempt.
	// Saturation can drop scheduled seed jobs, which must not silently turn a
	// "10,000 groups" case into a much smaller database.
	if c.Groups > c.Outputs && c.Preload >= c.Groups/c.Outputs {
		covered := map[int]bool{}
		f, err := os.Open(filepath.Join(out, "preload.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			var e pushLedgerEntry
			if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
				t.Fatal(err)
			}
			if e.Receipt != nil {
				covered[e.Producer] = true
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
		for i, p := range lab.producers {
			if !covered[i] {
				status, _ := scaleSend(t, lab, p, fmt.Sprintf("seed-group-%d", i), p.Body)
				if status != 202 {
					t.Fatal("group seed rejected", status)
				}
			}
		}
	}
	t.Logf("preload actual=%d target=%d", r.Accepted, c.Preload)
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
				tenant := lab.tenants[i%len(lab.tenants)]
				i++
				_, err := tenant.App.Inspect(ctx, "ci")
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
