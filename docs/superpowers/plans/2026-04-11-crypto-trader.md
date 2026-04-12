# Crypto Trader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a three-service automated crypto trading system: Market Poller (WebSocket feeds + technical indicators) -> Ding (YAML trading rules) -> Trade Executor (order placement, risk management, paper trading).

**Architecture:** Market Poller connects to exchange WebSocket APIs, computes technical indicators (SMA crossover, deviation, volatility), and POSTs derived metrics to Ding. Ding evaluates threshold rules and fires webhooks to the Trade Executor. The executor manages positions, enforces risk limits, and places orders (or simulates them in paper mode). Each service runs independently and communicates over HTTP.

**Tech Stack:** Go 1.24, `gorilla/websocket` for WebSocket clients, `gopkg.in/yaml.v3` for config, standard `net/http` for servers, no external frameworks.

**Spec:** `docs/superpowers/specs/2026-04-11-crypto-trader-design.md`

---

## File Structure

### Ding repo (prerequisite change)

- Modify: `internal/evaluator/condition.go` — add negative literal support to regex
- Modify: `internal/evaluator/evaluator_test.go` — add negative literal test cases

### crypto-trader repo (new, at `/Users/zuchka/code/crypto-trader/`)

```
crypto-trader/
├── go.mod
├── .gitignore
├── cmd/
│   ├── poller/
│   │   └── main.go              # Poller CLI entry point
│   └── executor/
│       └── main.go              # Executor CLI entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Shared config types (Duration wrapper, YAML loading)
│   ├── poller/
│   │   ├── indicators.go        # Indicator interface + SMA, SMA crossover, deviation, volatility
│   │   ├── indicators_test.go
│   │   ├── adapter.go           # ExchangeAdapter interface + Tick type
│   │   ├── binance.go           # Binance WebSocket adapter
│   │   ├── binance_test.go      # Binance message parsing tests
│   │   ├── poller.go            # Core polling loop: adapters -> indicators -> POST to Ding
│   │   └── poller_test.go
│   └── executor/
│       ├── signal.go            # Signal parsing (webhook payload -> action)
│       ├── signal_test.go
│       ├── position.go          # Position manager (open/close, persistence)
│       ├── position_test.go
│       ├── risk.go              # Risk manager (daily loss, trade count, kill switch)
│       ├── risk_test.go
│       ├── exchange.go          # Exchange interface (OrderPlacer)
│       ├── paper.go             # Paper trading implementation
│       ├── paper_test.go
│       ├── orders.go            # Order orchestration (signal -> risk check -> place order)
│       ├── orders_test.go
│       ├── monitor.go           # Position monitoring (stop-loss, take-profit, max-hold)
│       ├── monitor_test.go
│       ├── server.go            # HTTP server (/signal, /status, /positions, /kill, etc.)
│       ├── server_test.go
│       ├── tradelog.go          # JSONL trade log writer
│       └── tradelog_test.go
├── configs/
│   ├── poller.yaml.example
│   ├── executor.yaml.example
│   └── ding-trading.yaml.example
└── trade-log/                   # gitignored
```

---

## Phase 1: Ding Prerequisite

### Task 1: Add negative literal support to Ding's condition parser

**Files:**
- Modify: `/Users/zuchka/code/ding/internal/evaluator/condition.go:12-13`
- Modify: `/Users/zuchka/code/ding/internal/evaluator/evaluator_test.go:14-39`

- [ ] **Step 1: Write failing tests for negative literals**

Add test cases to the existing `TestParseCondition_EventPerEvent` table in `evaluator_test.go`:

```go
// Add to the cases slice in TestParseCondition_EventPerEvent:
{"value > -5", ">", -5},
{"value < -0.5", "<", -0.5},
{"value >= -100.25", ">=", -100.25},
```

Add a new test for windowed negative literals:

```go
func TestParseCondition_WindowedNegative(t *testing.T) {
	c, err := evaluator.ParseCondition("avg(value) over 5m > -10.5")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Windowed {
		t.Fatal("expected windowed condition")
	}
	if c.Func != "avg" || c.Window != 5*time.Minute || c.Op != ">" || c.Literal != -10.5 {
		t.Errorf("unexpected condition: %+v", c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/ding && go test ./internal/evaluator/ -run "TestParseCondition" -v`
Expected: FAIL — negative literal cases return "unrecognized condition syntax"

- [ ] **Step 3: Fix the regex to support negative literals**

In `condition.go` lines 12-13, change `(\d+(?:\.\d+)?)` to `(-?\d+(?:\.\d+)?)`:

```go
var (
	reEventCond    = regexp.MustCompile(`^value\s*(>|>=|<|<=|==|!=)\s*(-?\d+(?:\.\d+)?)$`)
	reWindowedCond = regexp.MustCompile(`^(avg|max|min|count|sum)\(value\)\s+over\s+(\d+[smh])\s*(>|>=|<|<=|==|!=)\s*(-?\d+(?:\.\d+)?)$`)
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zuchka/code/ding && go test ./internal/evaluator/ -run "TestParseCondition" -v`
Expected: PASS — all existing + new tests pass

- [ ] **Step 5: Run full test suite to verify no regressions**

Run: `cd /Users/zuchka/code/ding && go test ./...`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
cd /Users/zuchka/code/ding
git add internal/evaluator/condition.go internal/evaluator/evaluator_test.go
git commit -m "feat: support negative number literals in condition parser

Conditions like 'value < -0.5' and 'avg(value) over 5m > -10.5'
now parse correctly. Previously the regex only matched non-negative
numbers."
```

---

## Phase 2: Poller Foundation

### Task 2: Scaffold the crypto-trader project

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/go.mod`
- Create: `/Users/zuchka/code/crypto-trader/.gitignore`

- [ ] **Step 1: Initialize the project**

```bash
mkdir -p /Users/zuchka/code/crypto-trader
cd /Users/zuchka/code/crypto-trader
git init
go mod init github.com/zuchka/crypto-trader
mkdir -p cmd/poller cmd/executor internal/config internal/poller internal/executor configs trade-log
```

- [ ] **Step 2: Create .gitignore**

```
# Binaries
/cmd/poller/poller
/cmd/executor/executor

# Config (may contain secrets)
*.yaml
!configs/*.yaml.example

# Trade logs
/trade-log/*.jsonl

# Go
*.exe
*.test
*.out
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: scaffold crypto-trader project"
```

### Task 3: Shared config types

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/config/config.go`

- [ ] **Step 1: Write the Duration wrapper and YAML loader**

```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration for YAML unmarshaling of strings like "5m", "500ms".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// LoadFile reads and unmarshals a YAML file into dst.
func LoadFile(path string, dst interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 2: Install yaml dependency and verify it compiles**

```bash
cd /Users/zuchka/code/crypto-trader
go get gopkg.in/yaml.v3
go build ./internal/config/
```

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go go.mod go.sum
git commit -m "feat: add shared config types (Duration wrapper, YAML loader)"
```

### Task 4: Technical indicators

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/indicators.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/indicators_test.go`

- [ ] **Step 1: Write failing tests for SMA indicator**

```go
package poller

import (
	"testing"
	"time"
)

func TestSMA_BasicAverage(t *testing.T) {
	sma := NewSMA(10 * time.Second)
	now := time.Now()
	sma.Push(10, now)
	sma.Push(20, now.Add(1 * time.Second))
	sma.Push(30, now.Add(2 * time.Second))

	got := sma.Value()
	want := 20.0
	if got != want {
		t.Errorf("SMA.Value() = %f, want %f", got, want)
	}
}

func TestSMA_ExpiresOldValues(t *testing.T) {
	sma := NewSMA(5 * time.Second)
	base := time.Now()
	sma.Push(100, base)                     // will expire
	sma.Push(10, base.Add(6*time.Second))   // within window
	sma.Push(20, base.Add(7*time.Second))   // within window

	got := sma.Value()
	want := 15.0
	if got != want {
		t.Errorf("SMA.Value() = %f, want %f (old value should be expired)", got, want)
	}
}

func TestSMA_EmptyReturnsZero(t *testing.T) {
	sma := NewSMA(10 * time.Second)
	if got := sma.Value(); got != 0 {
		t.Errorf("empty SMA.Value() = %f, want 0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestSMA" -v`
Expected: FAIL — compilation error, types not defined

- [ ] **Step 3: Implement SMA indicator**

```go
package poller

import (
	"sync"
	"time"
)

// Indicator computes a derived metric from a stream of price ticks.
type Indicator interface {
	Name() string
	Push(price float64, ts time.Time)
	Value() float64
}

// --- SMA (Simple Moving Average) ---

type timedValue struct {
	value float64
	ts    time.Time
}

// SMA computes a simple moving average over a time window.
type SMA struct {
	window time.Duration
	mu     sync.Mutex
	buf    []timedValue
}

func NewSMA(window time.Duration) *SMA {
	return &SMA{window: window}
}

func (s *SMA) Name() string { return "sma" }

func (s *SMA) Push(price float64, ts time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, timedValue{value: price, ts: ts})
	s.evict(ts)
}

func (s *SMA) Value() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return 0
	}
	s.evict(s.buf[len(s.buf)-1].ts)
	var sum float64
	for _, v := range s.buf {
		sum += v.value
	}
	return sum / float64(len(s.buf))
}

func (s *SMA) evict(now time.Time) {
	cutoff := now.Add(-s.window)
	i := 0
	for i < len(s.buf) && s.buf[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.buf = s.buf[i:]
	}
}
```

- [ ] **Step 4: Run SMA tests to verify they pass**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestSMA" -v`
Expected: PASS

- [ ] **Step 5: Write failing tests for SMA Crossover indicator**

```go
func TestSMACrossover_BullishSignal(t *testing.T) {
	// Short SMA above long SMA -> positive score
	c := NewSMACrossover("momentum", 2*time.Second, 10*time.Second, 0.001)
	base := time.Now()
	// Fill long window with low values
	for i := 0; i < 10; i++ {
		c.Push(100, base.Add(time.Duration(i)*time.Second))
	}
	// Push high values to lift the short window
	c.Push(110, base.Add(10*time.Second))
	c.Push(112, base.Add(11*time.Second))

	val := c.Value()
	if val <= 0 {
		t.Errorf("expected positive (bullish) crossover score, got %f", val)
	}
	if val > 1 || val < -1 {
		t.Errorf("crossover score should be in [-1, 1], got %f", val)
	}
}

func TestSMACrossover_BearishSignal(t *testing.T) {
	c := NewSMACrossover("momentum", 2*time.Second, 10*time.Second, 0.001)
	base := time.Now()
	for i := 0; i < 10; i++ {
		c.Push(100, base.Add(time.Duration(i)*time.Second))
	}
	// Push low values to drop the short window
	c.Push(88, base.Add(10*time.Second))
	c.Push(86, base.Add(11*time.Second))

	val := c.Value()
	if val >= 0 {
		t.Errorf("expected negative (bearish) crossover score, got %f", val)
	}
}

func TestSMACrossover_BelowThresholdReturnsZero(t *testing.T) {
	c := NewSMACrossover("momentum", 2*time.Second, 10*time.Second, 0.05) // 5% threshold
	base := time.Now()
	for i := 0; i < 12; i++ {
		c.Push(100, base.Add(time.Duration(i)*time.Second)) // flat, no divergence
	}

	val := c.Value()
	if val != 0 {
		t.Errorf("expected 0 (below threshold), got %f", val)
	}
}
```

- [ ] **Step 6: Implement SMA Crossover**

```go
// SMACrossover computes short SMA vs long SMA divergence, normalized to [-1, 1].
// Positive = bullish (short above long), negative = bearish.
// Returns 0 if divergence is below threshold.
type SMACrossover struct {
	name      string
	shortSMA  *SMA
	longSMA   *SMA
	threshold float64 // minimum divergence ratio to register
}

func NewSMACrossover(name string, shortWindow, longWindow time.Duration, threshold float64) *SMACrossover {
	return &SMACrossover{
		name:      name,
		shortSMA:  NewSMA(shortWindow),
		longSMA:   NewSMA(longWindow),
		threshold: threshold,
	}
}

func (c *SMACrossover) Name() string { return c.name }

func (c *SMACrossover) Push(price float64, ts time.Time) {
	c.shortSMA.Push(price, ts)
	c.longSMA.Push(price, ts)
}

func (c *SMACrossover) Value() float64 {
	longVal := c.longSMA.Value()
	if longVal == 0 {
		return 0
	}
	shortVal := c.shortSMA.Value()
	divergence := (shortVal - longVal) / longVal

	if divergence > 0 && divergence < c.threshold {
		return 0
	}
	if divergence < 0 && divergence > -c.threshold {
		return 0
	}

	// Clamp to [-1, 1]. Typical divergence is 0-5%, so scale by 10x for sensitivity.
	score := divergence * 10
	if score > 1 {
		score = 1
	}
	if score < -1 {
		score = -1
	}
	return score
}
```

- [ ] **Step 7: Run crossover tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestSMACrossover" -v`
Expected: PASS

- [ ] **Step 8: Write failing tests for Deviation and Volatility indicators**

```go
func TestDeviation_BelowAverage(t *testing.T) {
	d := NewDeviation("dev", 10*time.Second)
	base := time.Now()
	for i := 0; i < 10; i++ {
		d.Push(100, base.Add(time.Duration(i)*time.Second))
	}
	d.Push(98, base.Add(10*time.Second)) // 2% below

	val := d.Value()
	if val >= 0 {
		t.Errorf("expected negative deviation, got %f", val)
	}
	// Should be approximately -2.0 (percent)
	if val > -1.5 || val < -2.5 {
		t.Errorf("expected deviation near -2.0%%, got %f", val)
	}
}

func TestVolatility_HighRange(t *testing.T) {
	v := NewVolatility("vol", 10*time.Second)
	base := time.Now()
	v.Push(100, base)
	v.Push(90, base.Add(1*time.Second))   // min
	v.Push(110, base.Add(2*time.Second))  // max
	v.Push(100, base.Add(3*time.Second))

	val := v.Value()
	// range=20, avg=100, volatility=20%
	if val < 19 || val > 21 {
		t.Errorf("expected volatility near 20%%, got %f", val)
	}
}
```

- [ ] **Step 9: Implement Deviation and Volatility indicators**

```go
// Deviation measures current price as percentage deviation from SMA.
// Output: -2.0 means "2% below average".
type Deviation struct {
	name string
	sma  *SMA
	last float64
}

func NewDeviation(name string, window time.Duration) *Deviation {
	return &Deviation{name: name, sma: NewSMA(window)}
}

func (d *Deviation) Name() string { return d.name }

func (d *Deviation) Push(price float64, ts time.Time) {
	d.sma.Push(price, ts)
	d.last = price
}

func (d *Deviation) Value() float64 {
	avg := d.sma.Value()
	if avg == 0 {
		return 0
	}
	return ((d.last - avg) / avg) * 100
}

// Volatility measures (max - min) / avg over window as a percentage.
type Volatility struct {
	name   string
	window time.Duration
	mu     sync.Mutex
	buf    []timedValue
}

func NewVolatility(name string, window time.Duration) *Volatility {
	return &Volatility{name: name, window: window}
}

func (v *Volatility) Name() string { return v.name }

func (v *Volatility) Push(price float64, ts time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.buf = append(v.buf, timedValue{value: price, ts: ts})
	v.evict(ts)
}

func (v *Volatility) Value() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.buf) == 0 {
		return 0
	}
	v.evict(v.buf[len(v.buf)-1].ts)
	min, max, sum := v.buf[0].value, v.buf[0].value, 0.0
	for _, tv := range v.buf {
		if tv.value < min {
			min = tv.value
		}
		if tv.value > max {
			max = tv.value
		}
		sum += tv.value
	}
	avg := sum / float64(len(v.buf))
	if avg == 0 {
		return 0
	}
	return ((max - min) / avg) * 100
}

func (v *Volatility) evict(now time.Time) {
	cutoff := now.Add(-v.window)
	i := 0
	for i < len(v.buf) && v.buf[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		v.buf = v.buf[i:]
	}
}
```

- [ ] **Step 10: Run all indicator tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -v`
Expected: All 8 tests pass

- [ ] **Step 11: Commit**

```bash
git add internal/poller/indicators.go internal/poller/indicators_test.go
git commit -m "feat: add technical indicators (SMA, crossover, deviation, volatility)"
```

### Task 5: Exchange adapter interface and Tick type

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/adapter.go`

- [ ] **Step 1: Define the adapter interface and Tick type**

```go
package poller

import (
	"context"
	"time"
)

// Tick represents a normalized price tick from an exchange.
type Tick struct {
	Pair     string  // e.g., "BTC/USDT"
	Price    float64
	Exchange string  // e.g., "binance"
	Time     time.Time
}

// ExchangeAdapter connects to an exchange WebSocket feed and produces normalized ticks.
type ExchangeAdapter interface {
	// Connect establishes a WebSocket connection for the given pairs.
	Connect(ctx context.Context, pairs []string) error
	// Subscribe returns a channel of normalized ticks. Blocks until Connect is called.
	Subscribe() (<-chan Tick, error)
	// Name returns the exchange name (e.g., "binance").
	Name() string
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/zuchka/code/crypto-trader && go build ./internal/poller/`
Expected: Compiles without error

- [ ] **Step 3: Commit**

```bash
git add internal/poller/adapter.go
git commit -m "feat: add ExchangeAdapter interface and Tick type"
```

### Task 6: Binance WebSocket adapter

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/binance.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/binance_test.go`

- [ ] **Step 1: Write test for Binance message parsing**

The Binance adapter will receive JSON from the WebSocket. Test the parsing logic independently from the connection.

```go
package poller

import (
	"testing"
)

func TestParseBinanceTrade(t *testing.T) {
	// Binance aggTrade stream message format
	raw := []byte(`{
		"e": "aggTrade",
		"s": "BTCUSDT",
		"p": "67432.50",
		"T": 1718234567000
	}`)

	tick, err := parseBinanceTrade(raw, "BTC/USDT")
	if err != nil {
		t.Fatal(err)
	}
	if tick.Price != 67432.50 {
		t.Errorf("price = %f, want 67432.50", tick.Price)
	}
	if tick.Pair != "BTC/USDT" {
		t.Errorf("pair = %s, want BTC/USDT", tick.Pair)
	}
	if tick.Exchange != "binance" {
		t.Errorf("exchange = %s, want binance", tick.Exchange)
	}
}

func TestParseBinanceTrade_InvalidPrice(t *testing.T) {
	raw := []byte(`{"e": "aggTrade", "s": "BTCUSDT", "p": "notanumber", "T": 1718234567000}`)
	_, err := parseBinanceTrade(raw, "BTC/USDT")
	if err == nil {
		t.Fatal("expected error for invalid price")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestParseBinance" -v`
Expected: FAIL — `parseBinanceTrade` not defined

- [ ] **Step 3: Implement Binance adapter**

```go
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// binanceTradeMsg is the JSON shape of a Binance aggTrade stream message.
type binanceTradeMsg struct {
	Event  string `json:"e"`
	Symbol string `json:"s"`
	Price  string `json:"p"`
	Time   int64  `json:"T"` // trade time in milliseconds
}

func parseBinanceTrade(raw []byte, pair string) (Tick, error) {
	var msg binanceTradeMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Tick{}, fmt.Errorf("unmarshal binance trade: %w", err)
	}
	price, err := strconv.ParseFloat(msg.Price, 64)
	if err != nil {
		return Tick{}, fmt.Errorf("parse price %q: %w", msg.Price, err)
	}
	return Tick{
		Pair:     pair,
		Price:    price,
		Exchange: "binance",
		Time:     time.UnixMilli(msg.Time),
	}, nil
}

// BinanceAdapter connects to Binance's WebSocket API for real-time trade data.
type BinanceAdapter struct {
	baseURL string // e.g., "wss://stream.binance.com:9443/ws"
	pairs   []string
	conn    *websocket.Conn
	ticks   chan Tick
}

func NewBinanceAdapter(baseURL string) *BinanceAdapter {
	return &BinanceAdapter{
		baseURL: baseURL,
		ticks:   make(chan Tick, 256),
	}
}

func (b *BinanceAdapter) Name() string { return "binance" }

func (b *BinanceAdapter) Connect(ctx context.Context, pairs []string) error {
	b.pairs = pairs

	// Build combined stream URL: wss://stream.binance.com:9443/ws/btcusdt@aggTrade/ethusdt@aggTrade
	var streams []string
	for _, pair := range pairs {
		// "BTC/USDT" -> "btcusdt"
		symbol := strings.ToLower(strings.ReplaceAll(pair, "/", ""))
		streams = append(streams, symbol+"@aggTrade")
	}
	u, _ := url.Parse(b.baseURL)
	u.Path += "/" + strings.Join(streams, "/")

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("binance websocket connect: %w", err)
	}
	b.conn = conn

	// Build symbol -> pair lookup
	symbolToPair := make(map[string]string)
	for _, pair := range pairs {
		symbol := strings.ToUpper(strings.ReplaceAll(pair, "/", ""))
		symbolToPair[symbol] = pair
	}

	// Read messages in background
	go func() {
		defer close(b.ticks)
		for {
			_, msg, err := b.conn.ReadMessage()
			if err != nil {
				return // connection closed
			}
			// Combined stream wraps messages in {"stream": "...", "data": {...}}
			var wrapper struct {
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(msg, &wrapper) == nil && wrapper.Data != nil {
				msg = wrapper.Data
			}

			var trade binanceTradeMsg
			if json.Unmarshal(msg, &trade) != nil {
				continue
			}
			pair, ok := symbolToPair[trade.Symbol]
			if !ok {
				continue
			}
			tick, err := parseBinanceTrade(msg, pair)
			if err != nil {
				continue
			}
			select {
			case b.ticks <- tick:
			default: // drop if buffer full
			}
		}
	}()

	return nil
}

func (b *BinanceAdapter) Subscribe() (<-chan Tick, error) {
	return b.ticks, nil
}
```

- [ ] **Step 4: Install gorilla/websocket and run tests**

```bash
cd /Users/zuchka/code/crypto-trader
go get github.com/gorilla/websocket
go test ./internal/poller/ -run "TestParseBinance" -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/poller/binance.go internal/poller/binance_test.go go.mod go.sum
git commit -m "feat: add Binance WebSocket adapter with trade message parsing"
```

### Task 7: Poller core loop

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/poller.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/poller/poller_test.go`

- [ ] **Step 1: Write test for the poller's indicator computation and Ding payload**

```go
package poller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockAdapter implements ExchangeAdapter for testing.
type mockAdapter struct {
	ticks chan Tick
}

func (m *mockAdapter) Name() string                                    { return "mock" }
func (m *mockAdapter) Connect(_ context.Context, _ []string) error     { return nil }
func (m *mockAdapter) Subscribe() (<-chan Tick, error)                  { return m.ticks, nil }

func TestPoller_EmitsIndicatorsToSink(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]interface{}

	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Body is newline-delimited JSON
		for _, line := range splitLines(body) {
			var m map[string]interface{}
			if json.Unmarshal(line, &m) == nil {
				mu.Lock()
				received = append(received, m)
				mu.Unlock()
			}
		}
		w.WriteHeader(200)
	}))
	defer sink.Close()

	ticks := make(chan Tick, 10)
	adapter := &mockAdapter{ticks: ticks}

	indicators := []Indicator{
		NewDeviation("test_dev", 1*time.Minute),
	}

	p := &Poller{
		DingURL:       sink.URL,
		FlushInterval: 50 * time.Millisecond,
		Adapters:      []ExchangeAdapter{adapter},
		Indicators:    map[string][]Indicator{"BTC/USDT": indicators},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go p.Run(ctx)

	now := time.Now()
	ticks <- Tick{Pair: "BTC/USDT", Price: 100, Exchange: "mock", Time: now}
	ticks <- Tick{Pair: "BTC/USDT", Price: 102, Exchange: "mock", Time: now.Add(10 * time.Millisecond)}

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected at least one indicator payload sent to Ding")
	}

	// Check structure
	last := received[len(received)-1]
	if last["metric"] != "test_dev" {
		t.Errorf("metric = %v, want test_dev", last["metric"])
	}
	if last["pair"] != "BTC/USDT" {
		t.Errorf("pair = %v, want BTC/USDT", last["pair"])
	}
	if _, ok := last["value"]; !ok {
		t.Error("missing 'value' field in payload")
	}
	if last["exchange"] != "mock" {
		t.Errorf("exchange = %v, want mock", last["exchange"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestPoller_Emits" -v`
Expected: FAIL — `Poller` type not defined

- [ ] **Step 3: Implement the Poller**

```go
package poller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Poller reads ticks from exchange adapters, computes indicators, and POSTs results to Ding.
type Poller struct {
	DingURL       string
	FlushInterval time.Duration
	Adapters      []ExchangeAdapter
	// Indicators keyed by pair: "BTC/USDT" -> [momentum, deviation, ...]
	Indicators map[string][]Indicator
	// PairExchange maps pair -> exchange name for payload metadata
	PairExchange map[string]string
	client       *http.Client
}

// indicatorPayload is the JSON sent to Ding's /ingest.
type indicatorPayload struct {
	Metric   string  `json:"metric"`
	Value    float64 `json:"value"`
	Pair     string  `json:"pair"`
	Exchange string  `json:"exchange"`
}

func (p *Poller) Run(ctx context.Context) error {
	if p.client == nil {
		p.client = &http.Client{Timeout: 5 * time.Second}
	}

	// Merge all adapter tick channels
	merged := make(chan Tick, 512)
	for _, adapter := range p.Adapters {
		ch, err := adapter.Subscribe()
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", adapter.Name(), err)
		}
		go func(c <-chan Tick) {
			for tick := range c {
				select {
				case merged <- tick:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	ticker := time.NewTicker(p.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case tick := <-merged:
			// Track which exchange provides each pair
			if _, ok := p.PairExchange[tick.Pair]; !ok {
				if p.PairExchange == nil {
					p.PairExchange = make(map[string]string)
				}
				p.PairExchange[tick.Pair] = tick.Exchange
			}
			// Push to all indicators for this pair
			if indicators, ok := p.Indicators[tick.Pair]; ok {
				for _, ind := range indicators {
					ind.Push(tick.Price, tick.Time)
				}
			}
		case <-ticker.C:
			p.flush()
		}
	}
}

func (p *Poller) flush() {
	var buf bytes.Buffer
	for pair, indicators := range p.Indicators {
		for _, ind := range indicators {
			payload := indicatorPayload{
				Metric:   ind.Name(),
				Value:    ind.Value(),
				Pair:     pair,
				Exchange: p.PairExchange[pair],
			}
			line, _ := json.Marshal(payload)
			buf.Write(line)
			buf.WriteByte('\n')
		}
	}
	if buf.Len() == 0 {
		return
	}
	resp, err := p.client.Post(p.DingURL, "application/x-ndjson", &buf)
	if err != nil {
		log.Printf("POST to Ding failed: %v", err)
		return
	}
	resp.Body.Close()
}

// splitLines splits a byte slice on newlines, skipping empty lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/poller/ -run "TestPoller" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/poller/poller.go internal/poller/poller_test.go
git commit -m "feat: add poller core loop (adapters -> indicators -> POST to Ding)"
```

### Task 8: Poller config and CLI entry point

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/cmd/poller/main.go`
- Create: `/Users/zuchka/code/crypto-trader/configs/poller.yaml.example`

- [ ] **Step 1: Define poller config structs**

Add to `cmd/poller/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zuchka/crypto-trader/internal/config"
	"github.com/zuchka/crypto-trader/internal/poller"
)

type PollerConfig struct {
	Poller struct {
		DingURL       string          `yaml:"ding_url"`
		FlushInterval config.Duration `yaml:"flush_interval"`
	} `yaml:"poller"`
	Exchanges []ExchangeConfig  `yaml:"exchanges"`
	Indicators []IndicatorConfig `yaml:"indicators"`
}

type ExchangeConfig struct {
	Name   string   `yaml:"name"`
	Pairs  []string `yaml:"pairs"`
	Stream string   `yaml:"stream"`
}

type IndicatorConfig struct {
	Name        string          `yaml:"name"`
	Type        string          `yaml:"type"`
	ShortWindow config.Duration `yaml:"short_window"`
	LongWindow  config.Duration `yaml:"long_window"`
	Window      config.Duration `yaml:"window"`
	Threshold   float64         `yaml:"threshold"`
}

func main() {
	cfgPath := flag.String("c", "poller.yaml", "config file path")
	flag.Parse()

	var cfg PollerConfig
	if err := config.LoadFile(*cfgPath, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Build adapters
	var adapters []poller.ExchangeAdapter
	allPairs := make(map[string]bool)
	for _, ex := range cfg.Exchanges {
		switch ex.Name {
		case "binance":
			adapters = append(adapters, poller.NewBinanceAdapter(ex.Stream))
		default:
			log.Fatalf("unknown exchange: %s", ex.Name)
		}
		for _, pair := range ex.Pairs {
			allPairs[pair] = true
		}
	}

	// Build indicators per pair
	indicatorMap := make(map[string][]poller.Indicator)
	for pair := range allPairs {
		for _, ic := range cfg.Indicators {
			var ind poller.Indicator
			switch ic.Type {
			case "sma_crossover":
				ind = poller.NewSMACrossover(ic.Name, ic.ShortWindow.Duration, ic.LongWindow.Duration, ic.Threshold)
			case "deviation_from_sma":
				ind = poller.NewDeviation(ic.Name, ic.Window.Duration)
			case "range_over_mean":
				ind = poller.NewVolatility(ic.Name, ic.Window.Duration)
			default:
				log.Fatalf("unknown indicator type: %s", ic.Type)
			}
			indicatorMap[pair] = append(indicatorMap[pair], ind)
		}
	}

	flushInterval := cfg.Poller.FlushInterval.Duration
	if flushInterval == 0 {
		flushInterval = 500 * time.Millisecond
	}

	p := &poller.Poller{
		DingURL:       cfg.Poller.DingURL,
		FlushInterval: flushInterval,
		Adapters:      adapters,
		Indicators:    indicatorMap,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect adapters
	for i, adapter := range adapters {
		if err := adapter.Connect(ctx, cfg.Exchanges[i].Pairs); err != nil {
			log.Fatalf("connect %s: %v", adapter.Name(), err)
		}
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down poller...")
		cancel()
	}()

	log.Printf("poller started, flushing to %s every %s", cfg.Poller.DingURL, flushInterval)
	if err := p.Run(ctx); err != nil {
		log.Fatalf("poller error: %v", err)
	}
}
```

- [ ] **Step 2: Create example config**

Write `configs/poller.yaml.example`:

```yaml
# poller.yaml.example — copy to poller.yaml and customize

poller:
  ding_url: "http://localhost:8080/ingest"
  flush_interval: 500ms

exchanges:
  - name: binance
    pairs:
      - BTC/USDT
      - ETH/USDT
      - SOL/USDT
    stream: wss://stream.binance.com:9443/ws

indicators:
  - name: momentum_score
    type: sma_crossover
    short_window: 2m
    long_window: 10m
    threshold: 0.005

  - name: mean_dev_pct
    type: deviation_from_sma
    window: 1h

  - name: volatility_pct
    type: range_over_mean
    window: 5m
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/zuchka/code/crypto-trader && go build ./cmd/poller/`
Expected: Compiles without error

- [ ] **Step 4: Commit**

```bash
git add cmd/poller/main.go configs/poller.yaml.example
git commit -m "feat: add poller CLI entry point and example config"
```

---

## Phase 3: Executor Foundation

### Task 9: Signal parsing

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/signal.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/signal_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"testing"
)

func TestParseSignal_Buy(t *testing.T) {
	cases := []struct {
		rule   string
		action Action
	}{
		{"momentum_buy", ActionBuy},
		{"mean_reversion_buy", ActionBuy},
		{"my_custom_buy", ActionBuy},
	}
	for _, tc := range cases {
		sig := Signal{Rule: tc.rule, Pair: "BTC/USDT", Price: 100}
		if got := sig.Action(); got != tc.action {
			t.Errorf("rule %q: got %v, want %v", tc.rule, got, tc.action)
		}
	}
}

func TestParseSignal_Sell(t *testing.T) {
	sig := Signal{Rule: "momentum_sell", Pair: "BTC/USDT", Price: 100}
	if got := sig.Action(); got != ActionSell {
		t.Errorf("got %v, want ActionSell", got)
	}
}

func TestParseSignal_Info(t *testing.T) {
	sig := Signal{Rule: "volatility_alert", Pair: "SOL/USDT", Price: 100}
	if got := sig.Action(); got != ActionInfo {
		t.Errorf("got %v, want ActionInfo", got)
	}
}

func TestParseSignalFromWebhook(t *testing.T) {
	body := []byte(`{"rule":"momentum_buy","metric":"momentum_score","value":0.7,"pair":"BTC/USDT","fired_at":"2026-04-11T14:23:01Z"}`)
	sig, err := ParseWebhookPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Rule != "momentum_buy" || sig.Pair != "BTC/USDT" {
		t.Errorf("unexpected signal: %+v", sig)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestParseSignal" -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Implement signal parsing**

```go
package executor

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Action int

const (
	ActionBuy  Action = iota
	ActionSell
	ActionInfo
)

func (a Action) String() string {
	switch a {
	case ActionBuy:
		return "buy"
	case ActionSell:
		return "sell"
	default:
		return "info"
	}
}

// Signal represents a parsed trading signal from a Ding webhook.
type Signal struct {
	Rule    string  `json:"rule"`
	Metric  string  `json:"metric"`
	Value   float64 `json:"value"`
	Pair    string  `json:"pair"`
	FiredAt string  `json:"fired_at"`
}

// Action determines the trading action from the rule name convention.
func (s Signal) Action() Action {
	if strings.HasSuffix(s.Rule, "_buy") {
		return ActionBuy
	}
	if strings.HasSuffix(s.Rule, "_sell") {
		return ActionSell
	}
	return ActionInfo
}

// ParseWebhookPayload parses a Ding webhook JSON body into a Signal.
func ParseWebhookPayload(body []byte) (Signal, error) {
	var sig Signal
	if err := json.Unmarshal(body, &sig); err != nil {
		return Signal{}, fmt.Errorf("parse webhook payload: %w", err)
	}
	if sig.Rule == "" {
		return Signal{}, fmt.Errorf("webhook payload missing 'rule' field")
	}
	return sig, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestParseSignal" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/signal.go internal/executor/signal_test.go
git commit -m "feat: add signal parsing (webhook payload -> trading action)"
```

### Task 10: Trade log writer

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/tradelog.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/tradelog_test.go`

- [ ] **Step 1: Write failing test**

```go
package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTradeLog_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	tl, err := NewTradeLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	tl.Log("signal_received", map[string]interface{}{
		"rule": "momentum_buy",
		"pair": "BTC/USDT",
		"price": 67432.50,
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("invalid JSONL line: %v", err)
	}
	if entry["event"] != "signal_received" {
		t.Errorf("event = %v, want signal_received", entry["event"])
	}
	if _, ok := entry["ts"]; !ok {
		t.Error("missing 'ts' field")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestTradeLog" -v`
Expected: FAIL

- [ ] **Step 3: Implement trade log**

```go
package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// TradeLog writes structured JSONL entries to a file.
type TradeLog struct {
	mu   sync.Mutex
	file *os.File
}

func NewTradeLog(path string) (*TradeLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open trade log: %w", err)
	}
	return &TradeLog{file: f}, nil
}

func (tl *TradeLog) Log(event string, fields map[string]interface{}) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	entry := make(map[string]interface{}, len(fields)+2)
	entry["ts"] = time.Now().UTC().Format(time.RFC3339)
	entry["event"] = event
	for k, v := range fields {
		entry[k] = v
	}

	line, _ := json.Marshal(entry)
	tl.file.Write(line)
	tl.file.Write([]byte("\n"))
}

func (tl *TradeLog) Close() error {
	return tl.file.Close()
}
```

- [ ] **Step 4: Run test**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestTradeLog" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/tradelog.go internal/executor/tradelog_test.go
git commit -m "feat: add JSONL trade log writer"
```

### Task 11: Position manager

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/position.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/position_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"testing"
	"time"
)

func TestPositionManager_OpenAndGet(t *testing.T) {
	pm := NewPositionManager()
	pos := Position{
		ID:         "p1",
		Pair:       "BTC/USDT",
		Side:       "long",
		EntryPrice: 67000,
		Quantity:   0.001,
		EntryTime:  time.Now(),
		StopLoss:   65660,   // 2% below
		TakeProfit: 69010,   // 3% above
	}
	pm.Open(pos)

	got, ok := pm.Get("BTC/USDT")
	if !ok {
		t.Fatal("expected position for BTC/USDT")
	}
	if got.EntryPrice != 67000 {
		t.Errorf("entry price = %f, want 67000", got.EntryPrice)
	}
}

func TestPositionManager_RejectsDuplicate(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT"})
	err := pm.Open(Position{ID: "p2", Pair: "BTC/USDT"})
	if err == nil {
		t.Fatal("expected error for duplicate pair position")
	}
}

func TestPositionManager_Close(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT", EntryPrice: 67000, Quantity: 0.001})

	closed, err := pm.Close("BTC/USDT", 68000)
	if err != nil {
		t.Fatal(err)
	}
	if closed.EntryPrice != 67000 {
		t.Errorf("closed entry = %f, want 67000", closed.EntryPrice)
	}

	_, ok := pm.Get("BTC/USDT")
	if ok {
		t.Fatal("position should be removed after close")
	}
}

func TestPositionManager_Count(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT"})
	pm.Open(Position{ID: "p2", Pair: "ETH/USDT"})
	if pm.Count() != 2 {
		t.Errorf("count = %d, want 2", pm.Count())
	}
}

func TestPositionManager_PnL(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT", EntryPrice: 100, Quantity: 1.0})

	pnl := pm.UnrealizedPnL("BTC/USDT", 105)
	if pnl != 5.0 {
		t.Errorf("pnl = %f, want 5.0", pnl)
	}

	pnl = pm.UnrealizedPnL("BTC/USDT", 95)
	if pnl != -5.0 {
		t.Errorf("pnl = %f, want -5.0", pnl)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestPositionManager" -v`
Expected: FAIL

- [ ] **Step 3: Implement position manager**

```go
package executor

import (
	"fmt"
	"sync"
	"time"
)

// Position represents an open trading position.
type Position struct {
	ID         string    `json:"id"`
	Pair       string    `json:"pair"`
	Side       string    `json:"side"` // "long"
	EntryPrice float64   `json:"entry_price"`
	Quantity   float64   `json:"quantity"`
	EntryTime  time.Time `json:"entry_time"`
	OrderID    string    `json:"order_id"`
	StopLoss   float64   `json:"stop_loss"`
	TakeProfit float64   `json:"take_profit"`
}

// PositionManager tracks open positions.
type PositionManager struct {
	mu        sync.RWMutex
	positions map[string]Position // keyed by pair
}

func NewPositionManager() *PositionManager {
	return &PositionManager{
		positions: make(map[string]Position),
	}
}

func (pm *PositionManager) Open(pos Position) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, exists := pm.positions[pos.Pair]; exists {
		return fmt.Errorf("position already open for %s", pos.Pair)
	}
	pm.positions[pos.Pair] = pos
	return nil
}

func (pm *PositionManager) Get(pair string) (Position, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pos, ok := pm.positions[pair]
	return pos, ok
}

func (pm *PositionManager) Close(pair string, exitPrice float64) (Position, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pos, ok := pm.positions[pair]
	if !ok {
		return Position{}, fmt.Errorf("no open position for %s", pair)
	}
	delete(pm.positions, pair)
	return pos, nil
}

func (pm *PositionManager) Count() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.positions)
}

func (pm *PositionManager) All() []Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]Position, 0, len(pm.positions))
	for _, pos := range pm.positions {
		result = append(result, pos)
	}
	return result
}

func (pm *PositionManager) UnrealizedPnL(pair string, currentPrice float64) float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pos, ok := pm.positions[pair]
	if !ok {
		return 0
	}
	return (currentPrice - pos.EntryPrice) * pos.Quantity
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestPositionManager" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/position.go internal/executor/position_test.go
git commit -m "feat: add position manager (open, close, P&L tracking)"
```

### Task 12: Risk manager and kill switch

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/risk.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/risk_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"testing"
)

func TestRisk_AllowsTrade(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		MaxOpenPositions: 3,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
	})

	pm := NewPositionManager()
	if err := r.Check(pm); err != nil {
		t.Errorf("should allow trade: %v", err)
	}
}

func TestRisk_BlocksMaxPositions(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		MaxOpenPositions: 2,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
	})
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT"})
	pm.Open(Position{ID: "p2", Pair: "ETH/USDT"})

	if err := r.Check(pm); err == nil {
		t.Fatal("should block: max positions reached")
	}
}

func TestRisk_BlocksMaxDailyTrades(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		MaxOpenPositions: 10,
		MaxDailyTrades:   2,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
	})
	pm := NewPositionManager()
	r.RecordTrade()
	r.RecordTrade()

	if err := r.Check(pm); err == nil {
		t.Fatal("should block: max daily trades reached")
	}
}

func TestRisk_KillSwitch(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		MaxOpenPositions: 10,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
	})
	pm := NewPositionManager()

	r.RecordPnL(-50)  // -5%
	if err := r.Check(pm); err != nil {
		t.Errorf("should still allow at -5%%: %v", err)
	}

	r.RecordPnL(-60)  // -11% total
	if err := r.Check(pm); err == nil {
		t.Fatal("should block: kill switch should be triggered at -11%")
	}
	if !r.IsKilled() {
		t.Fatal("kill switch should be active")
	}
}

func TestRisk_ManualKillAndResume(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		MaxOpenPositions: 10,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
	})
	pm := NewPositionManager()

	r.Kill()
	if err := r.Check(pm); err == nil {
		t.Fatal("should block after manual kill")
	}

	r.Resume()
	if err := r.Check(pm); err != nil {
		t.Errorf("should allow after resume: %v", err)
	}
}

func TestRisk_PositionSize(t *testing.T) {
	r := NewRiskManager(RiskConfig{
		PortfolioValue:  1000,
		PositionSizePct: 5,
	})
	size := r.PositionSize()
	if size != 50 {
		t.Errorf("position size = %f, want 50", size)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestRisk" -v`
Expected: FAIL

- [ ] **Step 3: Implement risk manager**

```go
package executor

import (
	"fmt"
	"sync"
)

type RiskConfig struct {
	MaxOpenPositions int
	MaxDailyTrades   int
	MaxDailyLossPct  float64
	PortfolioValue   float64
	PositionSizePct  float64
}

// RiskManager enforces trading limits and the kill switch.
type RiskManager struct {
	cfg         RiskConfig
	mu          sync.Mutex
	dailyTrades int
	dailyPnL    float64
	killed      bool
}

func NewRiskManager(cfg RiskConfig) *RiskManager {
	return &RiskManager{cfg: cfg}
}

// Check returns an error if a new trade should not be placed.
func (r *RiskManager) Check(pm *PositionManager) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.killed {
		return fmt.Errorf("kill switch active: trading halted")
	}
	if pm.Count() >= r.cfg.MaxOpenPositions {
		return fmt.Errorf("max open positions reached (%d)", r.cfg.MaxOpenPositions)
	}
	if r.dailyTrades >= r.cfg.MaxDailyTrades {
		return fmt.Errorf("max daily trades reached (%d)", r.cfg.MaxDailyTrades)
	}
	return nil
}

func (r *RiskManager) RecordTrade() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dailyTrades++
}

func (r *RiskManager) RecordPnL(pnl float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dailyPnL += pnl
	lossPct := (-r.dailyPnL / r.cfg.PortfolioValue) * 100
	if lossPct >= r.cfg.MaxDailyLossPct {
		r.killed = true
	}
}

func (r *RiskManager) IsKilled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.killed
}

func (r *RiskManager) Kill() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.killed = true
}

func (r *RiskManager) Resume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.killed = false
}

func (r *RiskManager) PositionSize() float64 {
	return r.cfg.PortfolioValue * r.cfg.PositionSizePct / 100
}

func (r *RiskManager) DailyPnL() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dailyPnL
}

func (r *RiskManager) DailyTrades() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dailyTrades
}

// ResetDaily resets daily counters. Called at UTC midnight.
func (r *RiskManager) ResetDaily() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dailyTrades = 0
	r.dailyPnL = 0
	r.killed = false
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestRisk" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/risk.go internal/executor/risk_test.go
git commit -m "feat: add risk manager with kill switch, daily limits, position sizing"
```

### Task 13: Exchange interface and paper trading

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/exchange.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/paper.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/paper_test.go`

- [ ] **Step 1: Write failing tests for paper trading**

```go
package executor

import (
	"testing"
)

func TestPaperExchange_PlaceOrder(t *testing.T) {
	pe := NewPaperExchange(1000)
	result, err := pe.PlaceOrder(Order{
		Pair:     "BTC/USDT",
		Side:     "buy",
		Type:     "limit",
		Price:    67000,
		Quantity: 0.001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "filled" {
		t.Errorf("status = %s, want filled", result.Status)
	}
	if result.FilledPrice != 67000 {
		t.Errorf("filled price = %f, want 67000", result.FilledPrice)
	}
}

func TestPaperExchange_TracksBallance(t *testing.T) {
	pe := NewPaperExchange(1000)
	pe.PlaceOrder(Order{
		Pair:     "BTC/USDT",
		Side:     "buy",
		Price:    100,
		Quantity: 2,
	})
	if pe.Balance() != 800 {
		t.Errorf("balance = %f, want 800 after buying 2*100", pe.Balance())
	}

	pe.PlaceOrder(Order{
		Pair:     "BTC/USDT",
		Side:     "sell",
		Price:    110,
		Quantity: 2,
	})
	if pe.Balance() != 1020 {
		t.Errorf("balance = %f, want 1020 after selling 2*110", pe.Balance())
	}
}

func TestPaperExchange_AppliesFee(t *testing.T) {
	pe := NewPaperExchange(1000)
	result, _ := pe.PlaceOrder(Order{
		Pair:     "BTC/USDT",
		Side:     "buy",
		Price:    1000,
		Quantity: 1,
	})
	// Default fee is 0.1%, so fee = 1.0
	if result.Fee < 0.9 || result.Fee > 1.1 {
		t.Errorf("fee = %f, want ~1.0 (0.1%% of 1000)", result.Fee)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestPaperExchange" -v`
Expected: FAIL

- [ ] **Step 3: Define exchange interface**

```go
package executor

// Order represents a trade order to be placed on an exchange.
type Order struct {
	Pair     string
	Side     string  // "buy" or "sell"
	Type     string  // "limit" or "market"
	Price    float64
	Quantity float64
}

// OrderResult is the exchange's response to a placed order.
type OrderResult struct {
	OrderID     string
	Status      string  // "filled", "cancelled", "pending"
	FilledPrice float64
	Quantity    float64
	Fee         float64
}

// Exchange abstracts order placement and cancellation.
type Exchange interface {
	PlaceOrder(order Order) (OrderResult, error)
	CancelOrder(orderID string) error
	GetPrice(pair string) (float64, error)
}
```

- [ ] **Step 4: Implement paper exchange**

```go
package executor

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// PaperExchange simulates an exchange for paper trading.
type PaperExchange struct {
	mu      sync.Mutex
	balance float64
	feePct  float64 // e.g., 0.001 for 0.1%
	nextID  atomic.Int64
	prices  map[string]float64
}

func NewPaperExchange(initialBalance float64) *PaperExchange {
	return &PaperExchange{
		balance: initialBalance,
		feePct:  0.001, // 0.1% per trade
		prices:  make(map[string]float64),
	}
}

func (pe *PaperExchange) PlaceOrder(order Order) (OrderResult, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	cost := order.Price * order.Quantity
	fee := cost * pe.feePct
	id := fmt.Sprintf("paper-%d", pe.nextID.Add(1))

	switch order.Side {
	case "buy":
		pe.balance -= cost + fee
	case "sell":
		pe.balance += cost - fee
	}

	pe.prices[order.Pair] = order.Price

	return OrderResult{
		OrderID:     id,
		Status:      "filled",
		FilledPrice: order.Price,
		Quantity:    order.Quantity,
		Fee:         fee,
	}, nil
}

func (pe *PaperExchange) CancelOrder(_ string) error {
	return nil // paper orders always fill instantly
}

func (pe *PaperExchange) GetPrice(pair string) (float64, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	price, ok := pe.prices[pair]
	if !ok {
		return 0, fmt.Errorf("no price data for %s", pair)
	}
	return price, nil
}

func (pe *PaperExchange) Balance() float64 {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	return pe.balance
}
```

- [ ] **Step 5: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestPaperExchange" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/executor/exchange.go internal/executor/paper.go internal/executor/paper_test.go
git commit -m "feat: add Exchange interface and paper trading implementation"
```

### Task 14: Order orchestration

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/orders.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/orders_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"testing"
	"time"
)

func TestOrderHandler_Buy(t *testing.T) {
	pm := NewPositionManager()
	rm := NewRiskManager(RiskConfig{
		MaxOpenPositions: 3,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
		PositionSizePct:  10,
	})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	oh := NewOrderHandler(pm, rm, ex, tl, OrderConfig{
		Type:            "limit",
		StopLossPct:     2.0,
		TakeProfitPct:   3.0,
		MaxHoldTime:     1 * time.Hour,
	})

	sig := Signal{Rule: "momentum_buy", Pair: "BTC/USDT", Value: 0.7}
	err := oh.HandleBuy(sig, 67000)
	if err != nil {
		t.Fatal(err)
	}

	pos, ok := pm.Get("BTC/USDT")
	if !ok {
		t.Fatal("expected open position for BTC/USDT")
	}
	if pos.EntryPrice != 67000 {
		t.Errorf("entry = %f, want 67000", pos.EntryPrice)
	}
	if pos.StopLoss != 67000*0.98 {
		t.Errorf("stop loss = %f, want %f", pos.StopLoss, 67000*0.98)
	}
}

func TestOrderHandler_BuyBlockedByRisk(t *testing.T) {
	pm := NewPositionManager()
	rm := NewRiskManager(RiskConfig{
		MaxOpenPositions: 0,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
		PositionSizePct:  10,
	})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	oh := NewOrderHandler(pm, rm, ex, tl, OrderConfig{
		StopLossPct:   2.0,
		TakeProfitPct: 3.0,
		MaxHoldTime:   1 * time.Hour,
	})

	err := oh.HandleBuy(Signal{Rule: "test_buy", Pair: "BTC/USDT"}, 67000)
	if err == nil {
		t.Fatal("expected risk check to block trade")
	}
}

func TestOrderHandler_Sell(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{ID: "p1", Pair: "BTC/USDT", EntryPrice: 67000, Quantity: 0.001})

	rm := NewRiskManager(RiskConfig{
		MaxOpenPositions: 3,
		MaxDailyTrades:   20,
		MaxDailyLossPct:  10,
		PortfolioValue:   1000,
		PositionSizePct:  10,
	})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	oh := NewOrderHandler(pm, rm, ex, tl, OrderConfig{})

	pnl, err := oh.HandleSell(Signal{Rule: "momentum_sell", Pair: "BTC/USDT"}, 68000)
	if err != nil {
		t.Fatal(err)
	}
	if pnl != (68000-67000)*0.001 {
		t.Errorf("pnl = %f, want %f", pnl, (68000-67000)*0.001)
	}
}

// newDiscardTradeLog returns a TradeLog that writes to /dev/null.
func newDiscardTradeLog() *TradeLog {
	tl, _ := NewTradeLog("/dev/null")
	return tl
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestOrderHandler" -v`
Expected: FAIL

- [ ] **Step 3: Implement order handler**

```go
package executor

import (
	"fmt"
	"time"

	"github.com/zuchka/crypto-trader/internal/config"
)

type OrderConfig struct {
	Type            string          // "limit" or "market"
	LimitOffsetPct  float64
	Timeout         config.Duration
	StopLossPct     float64
	TakeProfitPct   float64
	MaxHoldTime     time.Duration
}

// OrderHandler orchestrates the flow from signal to order execution.
type OrderHandler struct {
	positions *PositionManager
	risk      *RiskManager
	exchange  Exchange
	tradeLog  *TradeLog
	cfg       OrderConfig
	nextID    int
}

func NewOrderHandler(pm *PositionManager, rm *RiskManager, ex Exchange, tl *TradeLog, cfg OrderConfig) *OrderHandler {
	return &OrderHandler{
		positions: pm,
		risk:      rm,
		exchange:  ex,
		tradeLog:  tl,
		cfg:       cfg,
	}
}

func (oh *OrderHandler) HandleBuy(sig Signal, currentPrice float64) error {
	// Risk check
	if err := oh.risk.Check(oh.positions); err != nil {
		oh.tradeLog.Log("signal_blocked", map[string]interface{}{
			"rule": sig.Rule, "pair": sig.Pair, "reason": err.Error(),
		})
		return err
	}

	// Already have a position?
	if _, ok := oh.positions.Get(sig.Pair); ok {
		return fmt.Errorf("position already open for %s", sig.Pair)
	}

	// Calculate quantity
	positionValue := oh.risk.PositionSize()
	quantity := positionValue / currentPrice

	// Place order
	result, err := oh.exchange.PlaceOrder(Order{
		Pair:     sig.Pair,
		Side:     "buy",
		Type:     oh.cfg.Type,
		Price:    currentPrice,
		Quantity: quantity,
	})
	if err != nil {
		return fmt.Errorf("place buy order: %w", err)
	}

	oh.tradeLog.Log("order_filled", map[string]interface{}{
		"rule": sig.Rule, "pair": sig.Pair, "side": "buy",
		"price": result.FilledPrice, "qty": result.Quantity, "fee": result.Fee,
	})

	oh.nextID++
	pos := Position{
		ID:         fmt.Sprintf("pos-%d", oh.nextID),
		Pair:       sig.Pair,
		Side:       "long",
		EntryPrice: result.FilledPrice,
		Quantity:   result.Quantity,
		EntryTime:  time.Now(),
		OrderID:    result.OrderID,
		StopLoss:   result.FilledPrice * (1 - oh.cfg.StopLossPct/100),
		TakeProfit: result.FilledPrice * (1 + oh.cfg.TakeProfitPct/100),
	}
	oh.positions.Open(pos)
	oh.risk.RecordTrade()

	return nil
}

func (oh *OrderHandler) HandleSell(sig Signal, currentPrice float64) (float64, error) {
	pos, ok := oh.positions.Get(sig.Pair)
	if !ok {
		return 0, fmt.Errorf("no open position for %s", sig.Pair)
	}

	result, err := oh.exchange.PlaceOrder(Order{
		Pair:     sig.Pair,
		Side:     "sell",
		Type:     oh.cfg.Type,
		Price:    currentPrice,
		Quantity: pos.Quantity,
	})
	if err != nil {
		return 0, fmt.Errorf("place sell order: %w", err)
	}

	oh.positions.Close(sig.Pair, result.FilledPrice)
	pnl := (result.FilledPrice - pos.EntryPrice) * pos.Quantity

	oh.tradeLog.Log("position_closed", map[string]interface{}{
		"rule": sig.Rule, "pair": sig.Pair, "side": "sell",
		"entry": pos.EntryPrice, "exit": result.FilledPrice,
		"pnl": pnl, "fee": result.Fee,
	})

	oh.risk.RecordPnL(pnl)
	oh.risk.RecordTrade()

	return pnl, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestOrderHandler" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/orders.go internal/executor/orders_test.go
git commit -m "feat: add order handler (signal -> risk check -> exchange order -> position)"
```

### Task 15: Position monitor (stop-loss, take-profit, max-hold)

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/monitor.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/monitor_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"testing"
	"time"
)

func TestMonitor_TriggersStopLoss(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{
		ID: "p1", Pair: "BTC/USDT", EntryPrice: 100, Quantity: 1,
		StopLoss: 98, TakeProfit: 103, EntryTime: time.Now(),
	})

	rm := NewRiskManager(RiskConfig{PortfolioValue: 1000, MaxDailyLossPct: 50, MaxOpenPositions: 10, MaxDailyTrades: 100})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	mon := NewMonitor(pm, rm, ex, tl, 1*time.Hour)

	// Simulate price drop
	actions := mon.CheckPositions(map[string]float64{"BTC/USDT": 97})
	if len(actions) != 1 || actions[0].Reason != "stop_loss" {
		t.Errorf("expected stop_loss action, got %+v", actions)
	}
}

func TestMonitor_TriggersTakeProfit(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{
		ID: "p1", Pair: "ETH/USDT", EntryPrice: 100, Quantity: 1,
		StopLoss: 98, TakeProfit: 103, EntryTime: time.Now(),
	})

	rm := NewRiskManager(RiskConfig{PortfolioValue: 1000, MaxDailyLossPct: 50, MaxOpenPositions: 10, MaxDailyTrades: 100})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	mon := NewMonitor(pm, rm, ex, tl, 1*time.Hour)

	actions := mon.CheckPositions(map[string]float64{"ETH/USDT": 104})
	if len(actions) != 1 || actions[0].Reason != "take_profit" {
		t.Errorf("expected take_profit action, got %+v", actions)
	}
}

func TestMonitor_TriggersMaxHold(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{
		ID: "p1", Pair: "SOL/USDT", EntryPrice: 100, Quantity: 1,
		StopLoss: 90, TakeProfit: 110,
		EntryTime: time.Now().Add(-2 * time.Hour), // opened 2h ago
	})

	rm := NewRiskManager(RiskConfig{PortfolioValue: 1000, MaxDailyLossPct: 50, MaxOpenPositions: 10, MaxDailyTrades: 100})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	mon := NewMonitor(pm, rm, ex, tl, 1*time.Hour) // max hold = 1h

	actions := mon.CheckPositions(map[string]float64{"SOL/USDT": 101}) // price is fine, but time is up
	if len(actions) != 1 || actions[0].Reason != "max_hold_time" {
		t.Errorf("expected max_hold_time action, got %+v", actions)
	}
}

func TestMonitor_NoActionWhenInRange(t *testing.T) {
	pm := NewPositionManager()
	pm.Open(Position{
		ID: "p1", Pair: "BTC/USDT", EntryPrice: 100, Quantity: 1,
		StopLoss: 98, TakeProfit: 103, EntryTime: time.Now(),
	})

	rm := NewRiskManager(RiskConfig{PortfolioValue: 1000, MaxDailyLossPct: 50, MaxOpenPositions: 10, MaxDailyTrades: 100})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()

	mon := NewMonitor(pm, rm, ex, tl, 1*time.Hour)

	actions := mon.CheckPositions(map[string]float64{"BTC/USDT": 101}) // within range
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %+v", actions)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestMonitor" -v`
Expected: FAIL

- [ ] **Step 3: Implement monitor**

```go
package executor

import (
	"time"
)

// ExitAction describes why a position should be closed.
type ExitAction struct {
	Pair         string
	Reason       string  // "stop_loss", "take_profit", "max_hold_time"
	CurrentPrice float64
}

// Monitor checks open positions against current prices for exit conditions.
type Monitor struct {
	positions   *PositionManager
	risk        *RiskManager
	exchange    Exchange
	tradeLog    *TradeLog
	maxHoldTime time.Duration
}

func NewMonitor(pm *PositionManager, rm *RiskManager, ex Exchange, tl *TradeLog, maxHold time.Duration) *Monitor {
	return &Monitor{
		positions:   pm,
		risk:        rm,
		exchange:    ex,
		tradeLog:    tl,
		maxHoldTime: maxHold,
	}
}

// CheckPositions evaluates all open positions against current prices.
// Returns a list of positions that should be closed and the reason.
func (m *Monitor) CheckPositions(prices map[string]float64) []ExitAction {
	var actions []ExitAction
	now := time.Now()

	for _, pos := range m.positions.All() {
		price, ok := prices[pos.Pair]
		if !ok {
			continue
		}

		if price <= pos.StopLoss {
			actions = append(actions, ExitAction{Pair: pos.Pair, Reason: "stop_loss", CurrentPrice: price})
		} else if price >= pos.TakeProfit {
			actions = append(actions, ExitAction{Pair: pos.Pair, Reason: "take_profit", CurrentPrice: price})
		} else if now.Sub(pos.EntryTime) >= m.maxHoldTime {
			actions = append(actions, ExitAction{Pair: pos.Pair, Reason: "max_hold_time", CurrentPrice: price})
		}
	}
	return actions
}

// ExecuteExits closes positions from CheckPositions results.
func (m *Monitor) ExecuteExits(actions []ExitAction) {
	for _, action := range actions {
		pos, err := m.positions.Close(action.Pair, action.CurrentPrice)
		if err != nil {
			continue
		}

		result, err := m.exchange.PlaceOrder(Order{
			Pair:     action.Pair,
			Side:     "sell",
			Type:     "market",
			Price:    action.CurrentPrice,
			Quantity: pos.Quantity,
		})
		if err != nil {
			// Re-open position if sell fails
			m.positions.Open(pos)
			continue
		}

		pnl := (result.FilledPrice - pos.EntryPrice) * pos.Quantity
		m.risk.RecordPnL(pnl)

		m.tradeLog.Log(action.Reason, map[string]interface{}{
			"pair": action.Pair, "entry": pos.EntryPrice,
			"exit": result.FilledPrice, "pnl": pnl, "fee": result.Fee,
		})
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestMonitor" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/monitor.go internal/executor/monitor_test.go
git commit -m "feat: add position monitor (stop-loss, take-profit, max-hold-time)"
```

### Task 16: Executor HTTP server

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/server.go`
- Create: `/Users/zuchka/code/crypto-trader/internal/executor/server_test.go`

- [ ] **Step 1: Write failing tests**

```go
package executor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer() *Server {
	pm := NewPositionManager()
	rm := NewRiskManager(RiskConfig{
		MaxOpenPositions: 3, MaxDailyTrades: 20,
		MaxDailyLossPct: 10, PortfolioValue: 1000, PositionSizePct: 5,
	})
	ex := NewPaperExchange(1000)
	tl := newDiscardTradeLog()
	oh := NewOrderHandler(pm, rm, ex, tl, OrderConfig{
		StopLossPct: 2, TakeProfitPct: 3, MaxHoldTime: 1 * time.Hour,
	})
	mon := NewMonitor(pm, rm, ex, tl, 1*time.Hour)
	return NewServer(pm, rm, oh, mon, tl)
}

func TestServer_PostSignal_Buy(t *testing.T) {
	srv := newTestServer()
	// Seed a price so the exchange has data for BTC/USDT
	srv.orders.exchange.PlaceOrder(Order{Pair: "BTC/USDT", Side: "buy", Price: 67000, Quantity: 0})
	body := `{"rule":"momentum_buy","metric":"momentum_score","value":0.7,"pair":"BTC/USDT"}`
	req := httptest.NewRequest("POST", "/signal", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestServer_PostSignal_InfoOnly(t *testing.T) {
	srv := newTestServer()
	body := `{"rule":"volatility_alert","pair":"SOL/USDT","value":4.5}`
	req := httptest.NewRequest("POST", "/signal", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestServer_GetStatus(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var status map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &status)
	if _, ok := status["open_positions"]; !ok {
		t.Error("missing open_positions in status")
	}
	if _, ok := status["daily_pnl"]; !ok {
		t.Error("missing daily_pnl in status")
	}
}

func TestServer_PostKill(t *testing.T) {
	srv := newTestServer()

	// Kill
	req := httptest.NewRequest("POST", "/kill", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("kill status = %d", w.Code)
	}

	// Buy should fail
	body := `{"rule":"test_buy","pair":"BTC/USDT","value":1}`
	req = httptest.NewRequest("POST", "/signal", strings.NewReader(body))
	w = httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("signal status = %d", w.Code)
	}
	// The response should indicate the signal was blocked
}

func TestServer_PostResume(t *testing.T) {
	srv := newTestServer()

	// Kill then resume
	httptest.NewRequest("POST", "/kill", nil)
	srv.Mux().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/kill", nil))
	srv.Mux().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/resume", nil))

	// Should be able to trade again
	body := `{"rule":"test_buy","pair":"BTC/USDT","value":1}`
	req := httptest.NewRequest("POST", "/signal", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d after resume, want 200", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestServer" -v`
Expected: FAIL

- [ ] **Step 3: Implement server**

```go
package executor

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// Server is the executor's HTTP server.
type Server struct {
	positions *PositionManager
	risk      *RiskManager
	orders    *OrderHandler
	monitor   *Monitor
	tradeLog  *TradeLog
	mux       *http.ServeMux
}

func NewServer(pm *PositionManager, rm *RiskManager, oh *OrderHandler, mon *Monitor, tl *TradeLog) *Server {
	s := &Server{
		positions: pm,
		risk:      rm,
		orders:    oh,
		monitor:   mon,
		tradeLog:  tl,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("POST /signal", s.handleSignal)
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /positions", s.handlePositions)
	s.mux.HandleFunc("GET /history", s.handleHistory)
	s.mux.HandleFunc("POST /kill", s.handleKill)
	s.mux.HandleFunc("POST /resume", s.handleResume)
	return s
}

func (s *Server) Mux() *http.ServeMux { return s.mux }

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), 400)
		return
	}

	sig, err := ParseWebhookPayload(body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	s.tradeLog.Log("signal_received", map[string]interface{}{
		"rule": sig.Rule, "pair": sig.Pair, "value": sig.Value,
	})

	action := sig.Action()
	switch action {
	case ActionBuy:
		price, err := s.orders.exchange.GetPrice(sig.Pair)
		if err != nil {
			log.Printf("buy skipped: no price for %s: %v", sig.Pair, err)
			break
		}
		if err := s.orders.HandleBuy(sig, price); err != nil {
			log.Printf("buy blocked: %v", err)
		}
	case ActionSell:
		price, err := s.orders.exchange.GetPrice(sig.Pair)
		if err != nil {
			log.Printf("sell skipped: no price for %s: %v", sig.Pair, err)
			break
		}
		if _, err := s.orders.HandleSell(sig, price); err != nil {
			log.Printf("sell skipped: %v", err)
		}
	case ActionInfo:
		log.Printf("info signal: %s %s value=%.4f", sig.Rule, sig.Pair, sig.Value)
	}

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	status := map[string]interface{}{
		"open_positions": s.positions.Count(),
		"daily_pnl":     s.risk.DailyPnL(),
		"daily_trades":  s.risk.DailyTrades(),
		"kill_switch":   s.risk.IsKilled(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handlePositions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.positions.All())
}

func (s *Server) handleHistory(w http.ResponseWriter, _ *http.Request) {
	// Placeholder — would read from trade log file
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

func (s *Server) handleKill(w http.ResponseWriter, _ *http.Request) {
	s.risk.Kill()
	log.Println("KILL SWITCH ACTIVATED — all trading halted")
	s.tradeLog.Log("kill_switch", map[string]interface{}{"triggered_by": "manual"})
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
}

func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	s.risk.Resume()
	log.Println("Trading resumed")
	s.tradeLog.Log("resume", map[string]interface{}{})
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]string{"status": "resumed"})
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./internal/executor/ -run "TestServer" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/executor/server.go internal/executor/server_test.go
git commit -m "feat: add executor HTTP server (/signal, /status, /kill, /resume)"
```

### Task 17: Executor CLI entry point and example configs

**Files:**
- Create: `/Users/zuchka/code/crypto-trader/cmd/executor/main.go`
- Create: `/Users/zuchka/code/crypto-trader/configs/executor.yaml.example`
- Create: `/Users/zuchka/code/crypto-trader/configs/ding-trading.yaml.example`

- [ ] **Step 1: Implement executor CLI**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zuchka/crypto-trader/internal/config"
	"github.com/zuchka/crypto-trader/internal/executor"
)

type ExecutorConfig struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Exchange struct {
		Name      string `yaml:"name"`
		APIKey    string `yaml:"api_key"`
		APISecret string `yaml:"api_secret"`
	} `yaml:"exchange"`
	Trading struct {
		Mode             string  `yaml:"mode"` // "paper" or "live"
		BaseCurrency     string  `yaml:"base_currency"`
		InitialBalance   float64 `yaml:"initial_balance"`
		PositionSizePct  float64 `yaml:"position_size_pct"`
		MaxOpenPositions int     `yaml:"max_open_positions"`
		MaxDailyLossPct  float64 `yaml:"max_daily_loss_pct"`
		MaxDailyTrades   int     `yaml:"max_daily_trades"`
	} `yaml:"trading"`
	Orders struct {
		Type           string          `yaml:"type"`
		LimitOffsetPct float64         `yaml:"limit_offset_pct"`
		Timeout        config.Duration `yaml:"timeout"`
	} `yaml:"orders"`
	Positions struct {
		StopLossPct   float64         `yaml:"stop_loss_pct"`
		TakeProfitPct float64         `yaml:"take_profit_pct"`
		MaxHoldTime   config.Duration `yaml:"max_hold_time"`
	} `yaml:"positions"`
	Notifications struct {
		WebhookURL string   `yaml:"webhook_url"`
		Events     []string `yaml:"events"`
	} `yaml:"notifications"`
	TradeLog struct {
		Dir string `yaml:"dir"`
	} `yaml:"trade_log"`
}

func main() {
	cfgPath := flag.String("c", "executor.yaml", "config file path")
	flag.Parse()

	var cfg ExecutorConfig
	if err := config.LoadFile(*cfgPath, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 9090
	}
	if cfg.Trading.Mode == "" {
		cfg.Trading.Mode = "paper"
	}
	if cfg.Positions.MaxHoldTime.Duration == 0 {
		cfg.Positions.MaxHoldTime.Duration = 1 * time.Hour
	}
	if cfg.TradeLog.Dir == "" {
		cfg.TradeLog.Dir = "trade-log"
	}
	if cfg.Trading.InitialBalance == 0 {
		cfg.Trading.InitialBalance = 1000
	}

	// Trade log
	os.MkdirAll(cfg.TradeLog.Dir, 0755)
	logFile := filepath.Join(cfg.TradeLog.Dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	tl, err := executor.NewTradeLog(logFile)
	if err != nil {
		log.Fatalf("trade log: %v", err)
	}
	defer tl.Close()

	// Exchange
	var ex executor.Exchange
	switch cfg.Trading.Mode {
	case "paper":
		ex = executor.NewPaperExchange(cfg.Trading.InitialBalance)
		log.Println("PAPER TRADING MODE — no real orders will be placed")
	default:
		log.Fatalf("unsupported trading mode: %s (only 'paper' supported in v1)", cfg.Trading.Mode)
	}

	// Components
	pm := executor.NewPositionManager()
	rm := executor.NewRiskManager(executor.RiskConfig{
		MaxOpenPositions: cfg.Trading.MaxOpenPositions,
		MaxDailyTrades:   cfg.Trading.MaxDailyTrades,
		MaxDailyLossPct:  cfg.Trading.MaxDailyLossPct,
		PortfolioValue:   cfg.Trading.InitialBalance,
		PositionSizePct:  cfg.Trading.PositionSizePct,
	})
	oh := executor.NewOrderHandler(pm, rm, ex, tl, executor.OrderConfig{
		Type:          cfg.Orders.Type,
		StopLossPct:   cfg.Positions.StopLossPct,
		TakeProfitPct: cfg.Positions.TakeProfitPct,
		MaxHoldTime:   cfg.Positions.MaxHoldTime.Duration,
	})
	mon := executor.NewMonitor(pm, rm, ex, tl, cfg.Positions.MaxHoldTime.Duration)
	srv := executor.NewServer(pm, rm, oh, mon, tl)

	// HTTP server
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      srv.Mux(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down executor...")
		cancel()
		httpSrv.Shutdown(context.Background())
	}()

	// Position monitor loop
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prices := make(map[string]float64)
				for _, pos := range pm.All() {
					if p, err := ex.GetPrice(pos.Pair); err == nil {
						prices[pos.Pair] = p
					}
				}
				actions := mon.CheckPositions(prices)
				if len(actions) > 0 {
					mon.ExecuteExits(actions)
				}
			}
		}
	}()

	log.Printf("executor started on :%d (mode=%s)", cfg.Server.Port, cfg.Trading.Mode)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 2: Create executor.yaml.example**

```yaml
# executor.yaml.example — copy to executor.yaml and customize

server:
  port: 9090

exchange:
  name: binance
  # api_key: ${EXCHANGE_API_KEY}
  # api_secret: ${EXCHANGE_API_SECRET}

trading:
  mode: paper                    # "paper" or "live"
  base_currency: USDT
  initial_balance: 1000          # starting portfolio value in base currency
  position_size_pct: 5           # risk 5% of portfolio per trade
  max_open_positions: 3
  max_daily_loss_pct: 10         # kill switch threshold
  max_daily_trades: 20           # circuit breaker

orders:
  type: limit                    # "limit" or "market"
  limit_offset_pct: 0.02
  timeout: 30s

positions:
  stop_loss_pct: 2.0
  take_profit_pct: 3.0
  max_hold_time: 1h

trade_log:
  dir: trade-log

# notifications:
#   webhook_url: https://ntfy.sh/your-private-topic
#   events: [kill_switch, api_error, daily_summary]
```

- [ ] **Step 3: Create ding-trading.yaml.example**

```yaml
# ding-trading.yaml.example — Ding config for crypto trading signals
# Copy to ding-trading.yaml and customize

server:
  port: 8080
  format: json
  jq: '{metric: .metric, value: .value, pair: .pair, exchange: .exchange}'

notifiers:
  executor:
    type: webhook
    url: http://localhost:9090/signal
    max_attempts: 1

rules:
  # Momentum — poller emits "momentum_score" in range [-1, 1]
  - name: momentum_buy
    match:
      metric: momentum_score
      pair: BTC/USDT
    condition: "value > 0.5"
    cooldown: 5m
    message: "BTC momentum score {{ .value }} — bullish crossover"
    alert:
      - notifier: executor

  - name: momentum_sell
    match:
      metric: momentum_score
      pair: BTC/USDT
    condition: "value < -0.5"
    cooldown: 5m
    message: "BTC momentum score {{ .value }} — bearish crossover"
    alert:
      - notifier: executor

  # Mean reversion — poller emits "mean_dev_pct"
  - name: mean_reversion_buy
    match:
      metric: mean_dev_pct
      pair: ETH/USDT
    condition: "value < -2.0"
    cooldown: 15m
    message: "ETH {{ .value }}% below 1h average"
    alert:
      - notifier: executor

  - name: mean_reversion_sell
    match:
      metric: mean_dev_pct
      pair: ETH/USDT
    condition: "value > 2.0"
    cooldown: 15m
    message: "ETH {{ .value }}% above 1h average"
    alert:
      - notifier: executor

  # Volatility spike
  - name: volatility_alert
    match:
      metric: volatility_pct
      pair: SOL/USDT
    condition: "value > 3.0"
    cooldown: 10m
    message: "SOL 5m volatility {{ .value }}%"
    alert:
      - notifier: executor
```

- [ ] **Step 4: Verify everything compiles**

Run: `cd /Users/zuchka/code/crypto-trader && go build ./cmd/executor/ && go build ./cmd/poller/`
Expected: Both compile

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/zuchka/code/crypto-trader && go test ./... -v`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add cmd/executor/main.go configs/executor.yaml.example configs/ding-trading.yaml.example
git commit -m "feat: add executor CLI entry point and example configs

Includes executor.yaml.example and ding-trading.yaml.example
with momentum, mean reversion, and volatility trading rules."
```

---

## Verification

After all tasks are complete:

- [ ] **All tests pass:** `cd /Users/zuchka/code/crypto-trader && go test ./... -v`
- [ ] **Both binaries compile:** `go build ./cmd/poller/ && go build ./cmd/executor/`
- [ ] **Ding tests still pass with negative literal change:** `cd /Users/zuchka/code/ding && go test ./...`
- [ ] **Example configs are valid YAML:** review each `configs/*.yaml.example`
