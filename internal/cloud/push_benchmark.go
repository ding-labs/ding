package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

// Producer files contain environment variable names, never inline credentials.
// The ledger is synced before submission and after every attempt, so a crashed
// driver leaves enough information to reconcile uncertain acceptance.
type PushProducer struct {
	URL      string          `json:"url"`
	TokenEnv string          `json:"tokenEnv"`
	Body     json.RawMessage `json:"body"`
}
type PushBenchmarkOptions struct {
	Producers []PushProducer
	Duration  time.Duration
	Rate      int
	Ledger    string
	Client    *http.Client
}
type PushBenchmarkResult struct {
	GeneratorBusy   int     `json:"generatorBusy"`
	Offered         int     `json:"offered"`
	Accepted        int     `json:"accepted"`
	Replayed        int     `json:"replayed"`
	Rejected        int     `json:"rejected"`
	Unconfirmed     int     `json:"unconfirmed"`
	Attempts        int     `json:"attempts"`
	P95MS           float64 `json:"p95MS"`
	P99MS           float64 `json:"p99MS"`
	DurationSeconds float64 `json:"durationSeconds"`
}
type pushLedgerEntry struct {
	Key      string             `json:"key"`
	Producer int                `json:"producer"`
	Body     json.RawMessage    `json:"body,omitempty"`
	Stage    string             `json:"stage"`
	Status   int                `json:"status,omitempty"`
	At       time.Time          `json:"at"`
	Receipt  *store.PushReceipt `json:"receipt,omitempty"`
}

func BenchmarkPush(ctx context.Context, o PushBenchmarkOptions) (PushBenchmarkResult, error) {
	var result PushBenchmarkResult
	if len(o.Producers) == 0 || len(o.Producers) > 10000 || o.Rate < 1 || o.Rate > 1000 || o.Duration < time.Second || o.Duration > 7*24*time.Hour {
		return result, fmt.Errorf("provide bounded producers, duration (1s–7d), rate (1–1000/sec), and a new ledger file")
	}
	tokens := make([]string, len(o.Producers))
	for i, p := range o.Producers {
		u, err := url.Parse(p.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !json.Valid(p.Body) || len(p.Body) > 1<<20 {
			return result, fmt.Errorf("producer %d requires an HTTPS URL and bounded JSON", i)
		}
		tokens[i] = os.Getenv(p.TokenEnv)
		if tokens[i] == "" {
			return result, fmt.Errorf("producer %d token environment variable is unset", i)
		}
	}
	ledger, err := mcpconfig.CreatePrivate(o.Ledger)
	if err != nil {
		return result, err
	}
	defer ledger.Close()
	var ledgerMu sync.Mutex
	record := func(e pushLedgerEntry) error {
		ledgerMu.Lock()
		defer ledgerMu.Unlock()
		if err := json.NewEncoder(ledger).Encode(e); err != nil {
			return err
		}
		return ledger.Sync()
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		copy := *client
		if copy.Timeout == 0 {
			copy.Timeout = 15 * time.Second
		}
		copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &copy
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	deadline := start.Add(o.Duration)
	var mu sync.Mutex
	var failure error
	histogram := make([]int, 120001) // Bounded millisecond histogram, including retry latency.
	jobs := make(chan int, 32)
	var workers sync.WaitGroup
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if failure == nil {
			failure = err
		}
		cancel()
	}
	for worker := 0; worker < 16; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				p := o.Producers[i]
				key := state.ID()
				if err := record(pushLedgerEntry{Key: key, Producer: i, Body: p.Body, Stage: "offered", At: time.Now().UTC()}); err != nil {
					fail(err)
					continue
				}
				mu.Lock()
				result.Offered++
				mu.Unlock()
				accepted, rejected := false, false
				begin := time.Now()
				for attempt := 0; attempt < 4; attempt++ {
					req, err := http.NewRequestWithContext(run, "POST", p.URL, bytes.NewReader(p.Body))
					if err != nil {
						fail(err)
						break
					}
					req.Header.Set("Authorization", "Bearer "+tokens[i])
					req.Header.Set("Idempotency-Key", key)
					req.Header.Set("Content-Type", "application/json")
					mu.Lock()
					result.Attempts++
					mu.Unlock()
					response, err := client.Do(req)
					status := 0
					retryAfter := time.Duration(1<<attempt) * time.Second
					var receipt *store.PushReceipt
					if err == nil {
						status = response.StatusCode
						var envelope struct {
							Data store.PushReceipt `json:"data"`
						}
						decode := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope)
						response.Body.Close()
						if delay, e := time.ParseDuration(response.Header.Get("Retry-After") + "s"); e == nil && delay > retryAfter {
							retryAfter = min(delay, 30*time.Second)
						}
						mu.Lock()
						if status == 202 && decode == nil && envelope.Data.AcceptanceID != "" {
							receipt = &envelope.Data
							accepted = true
							result.Accepted++
							if receipt.Duplicate {
								result.Replayed++
							}
							histogram[min(int(time.Since(begin).Milliseconds()), len(histogram)-1)]++
						}
						if status >= 400 && status < 500 && status != 429 {
							rejected = true
							result.Rejected++
						}
						mu.Unlock()
					}
					if err := record(pushLedgerEntry{Key: key, Producer: i, Stage: "response", Status: status, At: time.Now().UTC(), Receipt: receipt}); err != nil {
						fail(err)
						break
					}
					if accepted || rejected || run.Err() != nil {
						break
					}
					// Stable body/key, bounded backoff and deterministic per-request jitter.
					retryAfter += time.Duration(key[attempt]) * time.Millisecond
					wait := time.NewTimer(retryAfter)
					select {
					case <-run.Done():
						wait.Stop()
					case <-wait.C:
					}
				}
				if !accepted && !rejected {
					mu.Lock()
					result.Unconfirmed++
					mu.Unlock()
				}
			}
		}()
	}
	tick := time.NewTicker(time.Second / time.Duration(o.Rate))
	defer tick.Stop()
	offered := 0
schedule:
	for time.Now().Before(deadline) {
		select {
		case <-run.Done():
			break schedule
		case <-tick.C:
		}
		select {
		case jobs <- offered % len(o.Producers):
			offered++
		default:
			mu.Lock()
			result.GeneratorBusy++
			mu.Unlock()
		}
	}
	close(jobs)
	workers.Wait()
	total := 0
	for _, n := range histogram {
		total += n
	}
	cumulative := 0
	p95Set := false
	for ms, n := range histogram {
		cumulative += n
		if !p95Set && cumulative >= (total*95+99)/100 {
			p95Set = true
			result.P95MS = float64(ms)
		}
		if cumulative >= (total*99+99)/100 {
			result.P99MS = float64(ms)
			break
		}
	}
	result.DurationSeconds = time.Since(start).Seconds()
	if failure != nil {
		return result, failure
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}
