package cloud

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

// Producer files contain environment variable names, never inline credentials.
// The default ledger syncs offers and responses. Preplanned mode syncs all
// possible keys before measurement and drains a bounded response journal at the
// end. Either mode leaves enough intent evidence to recover after a driver crash.
type PushProducer struct {
	URL      string          `json:"url"`
	TokenEnv string          `json:"tokenEnv"`
	Body     json.RawMessage `json:"body"`
}
type PushBenchmarkOptions struct {
	Producers        []PushProducer
	Duration         time.Duration
	Rate             int
	Ledger           string
	Client           *http.Client
	Workers          int
	NoRetry          bool
	LedgerBatch      int
	CompactLedger    bool
	PreplannedLedger bool // Persist every possible key before the measured clock starts.
}
type PushBenchmarkResult struct {
	BodyBytesMin        int            `json:"bodyBytesMin"`
	BodyBytesMax        int            `json:"bodyBytesMax"`
	WallDurationSeconds float64        `json:"wallDurationSeconds"`
	ClockDiscontinuity  bool           `json:"clockDiscontinuity"`
	GeneratorBusy       int            `json:"generatorBusy"`
	Offered             int            `json:"offered"`
	Accepted            int            `json:"accepted"`
	Replayed            int            `json:"replayed"`
	Rejected            int            `json:"rejected"`
	Unconfirmed         int            `json:"unconfirmed"`
	Attempts            int            `json:"attempts"`
	P95MS               float64        `json:"p95MS"`
	P99MS               float64        `json:"p99MS"`
	DurationSeconds     float64        `json:"durationSeconds"`
	FirstAccepted       int            `json:"firstAccepted"`
	Statuses            map[int]int    `json:"statuses"`
	Scheduled           int            `json:"scheduled"`
	ScheduleP99MS       float64        `json:"scheduleP99MS"`
	HTTPP99MS           float64        `json:"httpP99MS"`
	Throttled           int            `json:"throttled"`
	ErrorCodes          map[string]int `json:"errorCodes"`
	PreplannedLedger    bool           `json:"preplannedLedger"`
	JournalDrainSeconds float64        `json:"journalDrainSeconds"`
}
type pushLedgerEntry struct {
	Key         string             `json:"key"`
	Producer    int                `json:"producer"`
	Body        json.RawMessage    `json:"body,omitempty"`
	Stage       string             `json:"stage"`
	Status      int                `json:"status,omitempty"`
	At          time.Time          `json:"at"`
	Receipt     *store.PushReceipt `json:"receipt,omitempty"`
	ScheduledAt time.Time          `json:"scheduledAt,omitempty"`
	ErrorCode   string             `json:"errorCode,omitempty"`
}

func BenchmarkPush(ctx context.Context, o PushBenchmarkOptions) (PushBenchmarkResult, error) {
	var result PushBenchmarkResult
	if o.Workers == 0 {
		o.Workers = 16
	}
	if o.LedgerBatch == 0 {
		o.LedgerBatch = 1
	}
	if o.LedgerBatch < 1 || o.LedgerBatch > 1024 {
		return result, fmt.Errorf("ledger batch must be 1–1024")
	}
	if len(o.Producers) == 0 || len(o.Producers) > 10000 || o.Rate < 1 || o.Rate > 100000 || o.Workers < 1 || o.Workers > 1024 || o.Duration < time.Second || o.Duration > 7*24*time.Hour {
		return result, fmt.Errorf("provide bounded producers, duration (1s–7d), rate (1–100000/sec), workers (1–1024), and a new ledger file")
	}
	result.Statuses = map[int]int{}
	result.ErrorCodes = map[string]int{}
	tokens := make([]string, len(o.Producers))
	for i, p := range o.Producers {
		if i == 0 || len(p.Body) < result.BodyBytesMin {
			result.BodyBytesMin = len(p.Body)
		}
		result.BodyBytesMax = max(result.BodyBytesMax, len(p.Body))
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
	if o.CompactLedger {
		buffer := bufio.NewWriter(ledger)
		encoder := json.NewEncoder(buffer)
		for i, p := range o.Producers {
			if err := encoder.Encode(pushLedgerEntry{Producer: i, Body: p.Body, Stage: "producer", At: time.Now().UTC()}); err != nil {
				return result, err
			}
		}
		if err := buffer.Flush(); err != nil {
			return result, err
		}
		if err := ledger.Sync(); err != nil {
			return result, err
		}
	}
	totalScheduled := int(float64(o.Duration) / float64(time.Second) * float64(o.Rate))
	planID := ""
	var record func(pushLedgerEntry) error
	var drain func() error
	if o.PreplannedLedger {
		if !o.CompactLedger || totalScheduled > 1000000 {
			return result, fmt.Errorf("preplanned journal requires compact producers and at most one million arrivals")
		}
		planID = state.ID()
		buffer := bufio.NewWriter(ledger)
		encoder := json.NewEncoder(buffer)
		for i := range totalScheduled {
			if err := encoder.Encode(pushLedgerEntry{Stage: "planned", Key: planID + "-" + strconv.Itoa(i), Producer: i % len(o.Producers)}); err != nil {
				return result, err
			}
		}
		if err := buffer.Flush(); err != nil {
			return result, err
		}
		if err := ledger.Sync(); err != nil {
			return result, err
		}
		record, drain = plannedPushJournal(ledger, 4096)
		result.PreplannedLedger = true
	} else {
		var closeLedger func()
		record, closeLedger = groupedPushLedger(ledger, o.LedgerBatch)
		drain = func() error { closeLedger(); return nil }
	}
	var drainOnce sync.Once
	var drainErr error
	finishLedger := func() error { drainOnce.Do(func() { drainErr = drain() }); return drainErr }
	defer finishLedger()
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
	var mu sync.Mutex
	var failure error
	histogram := make([]int, 120001) // Bounded millisecond histogram, including retry latency.
	scheduleHistogram := make([]int, 120001)
	httpHistogram := make([]int, 120001)
	type job struct {
		producer int
		index    int
		due      time.Time
	}
	jobs := make(chan job, o.Workers*2)
	var workers sync.WaitGroup
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if failure == nil {
			failure = err
		}
		cancel()
	}
	for worker := 0; worker < o.Workers; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := range jobs {
				i := j.producer
				p := o.Producers[i]
				key := state.ID()
				if o.PreplannedLedger {
					key = planID + "-" + strconv.Itoa(j.index)
				}
				body := p.Body
				if o.CompactLedger {
					body = nil
				}
				if !o.PreplannedLedger {
					if err := record(pushLedgerEntry{Key: key, Producer: i, Body: body, Stage: "offered", At: time.Now().UTC(), ScheduledAt: j.due}); err != nil {
						fail(err)
						continue
					}
				}
				mu.Lock()
				result.Offered++
				mu.Unlock()
				accepted, rejected := false, false
				lastStatus := 0
				mu.Lock()
				scheduleHistogram[min(int(time.Since(j.due).Milliseconds()), len(scheduleHistogram)-1)]++
				mu.Unlock()
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
					begin := time.Now()
					response, err := client.Do(req)
					status := 0
					retryAfter := time.Duration(1<<attempt) * time.Second
					var receipt *store.PushReceipt
					code := ""
					if err == nil {
						status = response.StatusCode
						var envelope struct {
							Data  store.PushReceipt `json:"data"`
							Error struct {
								Code string `json:"code"`
							} `json:"error"`
						}
						decode := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope)
						response.Body.Close()
						if delay, e := time.ParseDuration(response.Header.Get("Retry-After") + "s"); e == nil && delay > retryAfter {
							retryAfter = min(delay, 30*time.Second)
						}
						code = envelope.Error.Code
						mu.Lock()
						if status == 202 && decode == nil && envelope.Data.AcceptanceID != "" {
							receipt = &envelope.Data
							accepted = true
							result.Accepted++
							if attempt == 0 {
								result.FirstAccepted++
							}
							if receipt.Duplicate {
								result.Replayed++
							}
							histogram[min(int(time.Since(j.due).Milliseconds()), len(histogram)-1)]++
						}
						if status >= 400 && status < 500 && status != 429 {
							rejected = true
							result.Rejected++
						}
						mu.Unlock()
					}
					lastStatus = status
					if err != nil {
						code = pushTransportCode(err)
					}
					mu.Lock()
					if code != "" && (len(code) > 128 || (len(result.ErrorCodes) >= 100 && result.ErrorCodes[code] == 0)) {
						code = "other"
					}
					if code != "" {
						result.ErrorCodes[code]++
					}
					result.Statuses[status]++
					httpHistogram[min(int(time.Since(begin).Milliseconds()), len(httpHistogram)-1)]++
					mu.Unlock()
					if err := record(pushLedgerEntry{Key: key, Producer: i, Stage: "response", Status: status, At: time.Now().UTC(), ScheduledAt: j.due, Receipt: receipt, ErrorCode: code}); err != nil {
						fail(err)
						break
					}
					if accepted || rejected || run.Err() != nil || o.NoRetry {
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
					if o.NoRetry && lastStatus == 429 {
						result.Throttled++
					} else {
						result.Unconfirmed++
					}
					mu.Unlock()
				}
			}
		}()
	}
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	timer.Stop()
schedule:
	for i := 0; i < totalScheduled; i++ {
		due := start.Add(time.Duration(float64(i+1) * float64(time.Second) / float64(o.Rate)))
		if delay := time.Until(due); delay > 0 {
			timer.Reset(delay)
			select {
			case <-run.Done():
				break schedule
			case <-timer.C:
			}
		}
		if run.Err() != nil {
			break
		}
		mu.Lock()
		result.Scheduled++
		mu.Unlock()
		select {
		case jobs <- job{producer: i % len(o.Producers), index: i, due: due}:
		default:
			mu.Lock()
			result.GeneratorBusy++
			mu.Unlock()
		}
	}
	close(jobs)
	workers.Wait()
	result.P95MS = histogramPercentile(histogram, 95)
	result.P99MS = histogramPercentile(histogram, 99)
	result.ScheduleP99MS = histogramPercentile(scheduleHistogram, 99)
	result.HTTPP99MS = histogramPercentile(httpHistogram, 99)
	end := time.Now()
	result.DurationSeconds = end.Sub(start).Seconds()
	result.WallDurationSeconds = end.UTC().Sub(start.UTC()).Seconds()
	result.ClockDiscontinuity = pushClockDiscontinuity(result.WallDurationSeconds, result.DurationSeconds)
	drainStart := time.Now()
	if err := finishLedger(); err != nil {
		return result, err
	}
	result.JournalDrainSeconds = time.Since(drainStart).Seconds()
	if failure != nil {
		return result, failure
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}

func pushClockDiscontinuity(wall, elapsed float64) bool {
	return math.Abs(wall-elapsed) > 2
}

// Error strings may contain URLs or credential material. Persist fixed classes.
func pushTransportCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "transport_cancelled"
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
		return "transport_timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "transport_eof"
	}
	if errors.Is(err, net.ErrClosed) {
		return "transport_closed"
	}
	return "transport_error"
}

func histogramPercentile(histogram []int, percentile int) float64 {
	total := 0
	for _, n := range histogram {
		total += n
	}
	if total == 0 {
		return 0
	}
	count := 0
	for ms, n := range histogram {
		count += n
		if count >= (total*percentile+99)/100 {
			return float64(ms)
		}
	}
	return 0
}
