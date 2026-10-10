package cloud

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/watchrun"
)

type BenchmarkOptions struct {
	Accounts          int
	Duration, Latency time.Duration
	BodyBytes         int
}
type BenchmarkResult struct {
	Accounts              int     `json:"accounts"`
	DurationSeconds       float64 `json:"durationSeconds"`
	Checks                int64   `json:"checks"`
	FirstObservationP95MS float64 `json:"firstObservationP95MS"`
	ObservedAccounts      int     `json:"observedAccounts"`
	HeapAllocBytes        uint64  `json:"heapAllocBytes"`
	RuntimeReservedBytes  uint64  `json:"runtimeReservedBytes"`
	Goroutines            int     `json:"goroutines"`
	LiveDiskBytes         int64   `json:"liveDiskBytes"`
	ClosedDiskBytes       int64   `json:"closedDiskBytes"`
	CloseMS               float64 `json:"closeMS"`
	Notes                 string  `json:"notes"`
}
type benchmarkHTTP struct {
	latency time.Duration
	body    string
	checks  atomic.Int64
}

func (b *benchmarkHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	timer := time.NewTimer(b.latency)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	b.checks.Add(1)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b.body)), Request: r}, nil
}

// Benchmark is a synthetic engine/storage benchmark with no external network.
// It is not an end-to-end authentication, TLS, provider-cost or 72-hour soak test.
func Benchmark(ctx context.Context, o BenchmarkOptions) (result BenchmarkResult, err error) {
	if o.Accounts < 1 || o.Accounts > MaxAccounts || o.Duration < time.Second || o.Duration > 24*time.Hour || o.Latency < 0 || o.Latency > 10*time.Second || o.BodyBytes < 0 || o.BodyBytes > 64<<10 {
		return result, fmt.Errorf("invalid benchmark bounds")
	}
	dir, err := os.MkdirTemp("", "ding-cloud-benchmark-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir)
	db, err := state.Open(ctx, dir)
	if err != nil {
		return result, err
	}
	defer db.Close()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return result, err
	}
	vault, _ := state.NewVault(db, key)
	transport := &benchmarkHTTP{latency: o.Latency, body: strings.Repeat("a", o.BodyBytes)}
	p := NewPool(ctx, dir, db, vault, &http.Client{Transport: transport})
	defer p.Close()
	var tenants []*Tenant
	started := map[string]time.Time{}
	manifest, err := Template(FirstWatch{ID: "health", URL: "https://benchmark.example/health", Destination: "webhook", Credential: "WEBHOOK"})
	if err != nil {
		return result, err
	}
	for i := 0; i < o.Accounts; i++ {
		a, err := db.Enroll(ctx, "https://synthetic-issuer.example", fmt.Sprint(i), o.Accounts)
		if err != nil {
			return result, err
		}
		t, err := p.Get(ctx, a.ID)
		if err != nil {
			return result, err
		}
		tenants = append(tenants, t)
		started[a.ID] = time.Now()
		if _, err := t.App.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
			return result, err
		}
	}
	begin := time.Now()
	timer := time.NewTimer(o.Duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case <-timer.C:
	}
	var latencies []float64
	for _, t := range tenants {
		records, err := t.App.List(ctx)
		if err != nil {
			return result, err
		}
		if len(records) > 0 && !records[0].LastInputAt.IsZero() {
			latencies = append(latencies, float64(records[0].LastInputAt.Sub(started[t.Account.ID]).Microseconds())/1000)
		}
	}
	sort.Float64s(latencies)
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result = BenchmarkResult{Accounts: o.Accounts, DurationSeconds: time.Since(begin).Seconds(), Checks: transport.checks.Load(), ObservedAccounts: len(latencies), HeapAllocBytes: memory.HeapAlloc, RuntimeReservedBytes: memory.Sys, Goroutines: runtime.NumGoroutine(), LiveDiskBytes: diskBytes(dir), Notes: "Synthetic healthy HTTP responses; five-minute cadence; one watch/account; memory is Go runtime allocation, not RSS. Short runs measure first observations, not sustained capacity, DNS/TLS, failures, provider costs or production availability."}
	if len(latencies) > 0 {
		index := (len(latencies)*95+99)/100 - 1
		result.FirstObservationP95MS = latencies[index]
	}
	closing := time.Now()
	if err := p.Close(); err != nil {
		return result, err
	}
	result.CloseMS = float64(time.Since(closing).Microseconds()) / 1000
	result.ClosedDiskBytes = diskBytes(dir)
	return result, nil
}

func diskBytes(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size
}

func WriteBenchmark(w io.Writer, result BenchmarkResult) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(result)
}
