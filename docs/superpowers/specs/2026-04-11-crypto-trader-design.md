# Crypto Trader: Automated Trading System Powered by Ding

**Date:** 2026-04-11
**Status:** Design approved

## Overview

A fully automated crypto trading system that uses Ding as the signal detection brain and a separate Trade Executor service for order placement. A Market Poller feeds real-time exchange data into Ding, which evaluates YAML-defined trading rules and fires webhooks to the executor when signals trigger.

The system targets 1-3% price swings on volatile crypto pairs, not micro-scalping. Starting capital is $500-2,000 (experimental "tuition money"). Exchange selection is a design variable, with Binance and Bybit as primary targets due to low fees (~0.2% round-trip).

## Architecture

Three Go services communicating over HTTP:

```
                        +---------------------+
                        |    Exchange APIs     |
                        |  (WebSocket feeds)   |
                        +------+----------^----+
                               |          |
                          prices      orders
                               |          |
                        +------v------+  +|---------------+
                        |   Market    |  |  Trade          |
                        |   Poller    |  |  Executor       |
                        +------+------+  +--^--------------+
                               |            |
                          POST /ingest   webhook
                               |            |
                        +------v------------|
                        |       Ding        |
                        +-------------------+
```

**Data flow:**

1. Market Poller connects to exchange WebSocket APIs, receives real-time price ticks
2. Computes technical indicators (moving averages, momentum scores, deviation percentages) from the raw tick stream
3. Emits derived metrics as Ding-compatible JSON: `{"metric": "momentum_score", "value": 0.7, "pair": "BTC/USDT", "exchange": "binance"}`
4. POSTs to Ding's `/ingest` endpoint
5. Ding's JQ transform extracts metric/value; labels (pair, exchange) pass through for rule matching
6. Rules evaluate simple thresholds against the pre-computed indicators (e.g., `value > 0.5` on `momentum_score`)
7. When a rule fires, Ding sends a webhook to the Trade Executor
8. Executor interprets the signal (rule name encodes intent), checks risk limits, places an order

**Key design decision:** Technical indicator computation (moving averages, cross-overs, deviation from mean) lives in the Poller, not in Ding's rule conditions. This is because Ding's condition parser supports `aggregate(value) over Xm > <number>` but not aggregate-vs-aggregate comparisons or arithmetic expressions. By computing indicators in Go code (testable, debuggable), Ding rules become simple threshold checks — which is exactly what Ding is good at.

**Key property:** Ding doesn't know it's trading. It evaluates rules on streaming numeric data and fires webhooks. Trading semantics live in rule names and the executor's interpretation.

## Component 1: Market Poller

A Go service that turns exchange WebSocket feeds into Ding-compatible JSON with pre-computed technical indicators.

### Configuration

```yaml
poller:
  ding_url: "http://localhost:8080/ingest"
  flush_interval: 500ms

exchanges:
  - name: binance
    pairs: [BTC/USDT, ETH/USDT, SOL/USDT]
    stream: wss://stream.binance.com:9443/ws

  - name: bybit
    pairs: [BTC/USDT, ETH/USDT]
    stream: wss://stream.bybit.com/v5/public/spot

indicators:
  - name: momentum_score
    type: sma_crossover        # short SMA vs long SMA, normalized to [-1, 1]
    short_window: 2m
    long_window: 10m
    threshold: 0.005           # 0.5% minimum divergence to register

  - name: mean_dev_pct
    type: deviation_from_sma   # current price as % deviation from SMA
    window: 1h

  - name: volatility_pct
    type: range_over_mean      # (max - min) / avg over window, as percentage
    window: 5m
```

### Design

- Connects to each exchange's public WebSocket API (no API keys needed)
- Subscribes to trade/ticker streams for configured pairs
- Normalizes exchange-specific JSON via per-exchange adapters
- **Computes technical indicators** from the raw tick stream using in-memory rolling windows
- Emits derived metrics to Ding: `{"metric": "momentum_score", "value": 0.7, "pair": "BTC/USDT", "exchange": "binance"}`
- Batches and POSTs to Ding on `flush_interval` (500ms default)
- Each exchange gets its own goroutine with reconnection logic

### Technical Indicator Types

| Type | Output | Description |
|---|---|---|
| `sma_crossover` | [-1, 1] | Short SMA vs long SMA, normalized. >0 = bullish, <0 = bearish. Magnitude indicates strength. |
| `deviation_from_sma` | percentage | Current price as % deviation from SMA. -2.1 means 2.1% below average. |
| `range_over_mean` | percentage | (max - min) / avg over window. Measures volatility as a percentage. |

Additional indicator types can be added as Go functions. Each implements a simple interface:

```go
type Indicator interface {
    Name() string
    Push(price float64, ts time.Time)
    Value() float64
}
```

### Exchange Adapter Interface

```go
type ExchangeAdapter interface {
    Connect(ctx context.Context, pairs []string) error
    Subscribe() (<-chan Tick, error)
    Name() string
}
```

Adding a new exchange means implementing this interface. Binance and Bybit are the first two targets.

### Scope boundaries

- No order placement (executor's job)
- No trade decision logic (Ding's job — the poller computes indicators, not signals)
- No API key management
- Maintains rolling window state for indicators, but this is ephemeral (rebuilt on restart from the live feed)

## Component 2: Ding Trading Rules

No code changes to Ding. Trading strategies are expressed as YAML rules using Ding's existing condition syntax. The Poller pre-computes technical indicators, so Ding rules are simple threshold checks.

### Example Configuration

```yaml
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
  # Momentum strategies — poller emits "momentum_score" in range [-1, 1]
  # positive = short SMA above long SMA (bullish), >0.5 = strong signal
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

  # Mean reversion — poller emits "mean_dev_pct" as % deviation from SMA
  # -2.0 means price is 2% below the 1h average
  - name: mean_reversion_buy
    match:
      metric: mean_dev_pct
      pair: ETH/USDT
    condition: "value < -2.0"
    cooldown: 15m
    message: "ETH {{ .value }}% below 1h average — buy the dip"
    alert:
      - notifier: executor

  - name: mean_reversion_sell
    match:
      metric: mean_dev_pct
      pair: ETH/USDT
    condition: "value > 2.0"
    cooldown: 15m
    message: "ETH {{ .value }}% above 1h average — take profit"
    alert:
      - notifier: executor

  # Volatility spike — poller emits "volatility_pct" as (max-min)/avg over window
  - name: volatility_alert
    match:
      metric: volatility_pct
      pair: SOL/USDT
    condition: "value > 3.0"
    cooldown: 10m
    message: "SOL 5m volatility {{ .value }}% — high volatility"
    alert:
      - notifier: executor
```

### Design choices

- **Rule names encode intent.** Executor parses rule names: `*_buy` -> buy, `*_sell` -> sell, anything else -> log only.
- **Cooldowns prevent overtrading.** First line of defense against fee burn.
- **Indicators pre-computed by Poller.** The Poller computes momentum scores, deviation percentages, and volatility metrics. Ding rules are simple threshold checks (`value > 0.5`), which matches Ding's condition parser exactly.
- **Thresholds embed fee awareness.** A momentum score of 0.5 corresponds to a ~0.5% SMA divergence, clearing the ~0.2% round-trip fee with margin. The exact mapping between score and price movement is controlled by the Poller's indicator config.
- **`max_attempts: 1`** on the notifier. Stale trading signals are worse than missed ones.
- **New strategies** are added by: (1) adding an indicator to the Poller config if needed, (2) adding a rule block to Ding's YAML, (3) hitting `/reload`. No restarts, no code changes to Ding.

### Limitation

Cross-pair correlations (e.g., "BTC moves but ETH doesn't follow") cannot be expressed as a single Ding rule. The Poller could potentially compute cross-pair indicators in the future. Not needed for v1.

## Component 3: Trade Executor

The most critical piece. A new Go service that handles order placement and risk management.

### Configuration

```yaml
server:
  port: 9090

exchange:
  name: binance
  api_key: ${EXCHANGE_API_KEY}
  api_secret: ${EXCHANGE_API_SECRET}

trading:
  mode: paper                    # "paper" or "live"
  base_currency: USDT
  position_size_pct: 5           # risk 5% of portfolio per trade
  max_open_positions: 3
  max_daily_loss_pct: 10         # kill switch threshold
  max_daily_trades: 20           # circuit breaker

orders:
  type: limit                    # "limit" or "market"
  limit_offset_pct: 0.02         # place limit 0.02% inside spread
  timeout: 30s                   # cancel unfilled limit orders after 30s
  fallback_to_market: false

positions:
  stop_loss_pct: 2.0
  take_profit_pct: 3.0
  max_hold_time: 1h

notifications:
  webhook_url: https://ntfy.sh/your-private-topic
  events: [kill_switch, api_error, daily_summary]
```

### Signal Interpretation

Receives Ding webhook payloads on `POST /signal`. Parses rule name to determine action:

| Rule name pattern | Action |
|---|---|
| `*_buy` | Open long position |
| `*_sell` | Close long position (skip if no position) |
| Anything else | Log only, no trade |

### Position Manager

Tracks open positions in memory, persisted to disk on interval:

```go
type Position struct {
    ID         string
    Pair       string
    Side       string     // "long"
    EntryPrice float64
    Quantity   float64
    EntryTime  time.Time
    OrderID    string     // exchange order ID
    StopLoss   float64
    TakeProfit float64
}
```

Pre-trade checks:
- No existing position for this pair
- Under `max_open_positions` limit
- Under `max_daily_trades` limit
- Kill switch not triggered

### Order Placement Flow

```
Signal received
  -> Check risk limits
  -> Calculate position size (portfolio_value * position_size_pct / 100)
  -> Place limit order slightly inside the spread
  -> Wait up to timeout for fill
  -> If filled: record position, set stop-loss/take-profit
  -> If not filled: cancel order, log, move on
```

### Position Monitoring

The executor maintains its own lightweight price feed for monitoring open positions. Since it already communicates with the exchange API for order placement, it subscribes to the exchange's WebSocket ticker for pairs with open positions. This is independent of the Poller's feed — the Poller feeds Ding for signal detection, while the executor's feed monitors positions for exit conditions.

A background goroutine checks all open positions against current prices every second:

- Price hits `stop_loss_pct` below entry -> close position
- Price hits `take_profit_pct` above entry -> close position
- Position age exceeds `max_hold_time` -> close position

### Kill Switch

The single most important safety feature. If daily realized losses exceed `max_daily_loss_pct`:

1. All open positions immediately closed
2. No new positions opened for rest of calendar day (UTC)
3. Notification webhook fires

### Paper Trading Mode

When `mode: paper`, the executor does everything except call the exchange API. Simulates fills at current price, tracks virtual positions, logs P&L. Validates strategies before risking real money.

### Persisted State

- Open positions
- Daily P&L running total
- Daily trade count
- Trade history log (JSONL)

### HTTP Endpoints

| Endpoint | Purpose |
|---|---|
| `GET /status` | Open positions, daily P&L, trade count, kill switch status |
| `GET /positions` | All open positions with unrealized P&L |
| `GET /history` | Today's closed trades |
| `POST /kill` | Manual kill switch |
| `POST /resume` | Resume trading after manual kill |
| `GET /metrics` | Prometheus metrics |

### Scope boundaries

- No strategy logic (Ding's job)
- No market data ingestion (Poller's job)
- No short selling in v1 (spot only)
- No leverage (spot only)
- No multi-exchange in v1 (one exchange per executor instance)

## Observability

### Trade Log (JSONL)

The executor writes every action to a structured trade log:

```json
{"ts":"2026-04-11T14:23:01Z","event":"signal_received","rule":"momentum_buy","pair":"BTC/USDT","price":67432.50}
{"ts":"2026-04-11T14:23:01Z","event":"order_placed","pair":"BTC/USDT","side":"buy","type":"limit","price":67419.02,"qty":0.00148}
{"ts":"2026-04-11T14:23:04Z","event":"order_filled","pair":"BTC/USDT","side":"buy","price":67419.02,"qty":0.00148,"fee":0.09}
{"ts":"2026-04-11T14:55:30Z","event":"take_profit","pair":"BTC/USDT","side":"sell","entry":67419.02,"exit":69441.59,"pnl":2.99,"pnl_pct":2.99}
{"ts":"2026-04-11T16:00:00Z","event":"daily_summary","trades":4,"wins":3,"losses":1,"pnl":5.23,"pnl_pct":0.52}
```

### Operational Notifications

The executor sends notifications for critical events via a simple webhook (not through Ding, to avoid circular dependency):
- Kill switch triggered
- Exchange API errors
- Executor crash/restart
- Daily P&L summary

### Existing Ding Observability

Ding's built-in endpoints provide signal-side monitoring:
- `/metrics` — event counts, alert counts per rule, webhook queue depth
- `/rules` — rule states and cooldown timers

## Project Structure

```
crypto-trader/
├── cmd/
│   ├── poller/
│   │   └── main.go
│   └── executor/
│       └── main.go
├── internal/
│   ├── poller/
│   │   ├── poller.go          # core polling loop
│   │   ├── adapter.go         # ExchangeAdapter interface
│   │   ├── binance.go         # Binance WebSocket adapter
│   │   ├── bybit.go           # Bybit WebSocket adapter
│   │   └── indicators.go     # technical indicator computation (SMA, crossover, deviation)
│   ├── executor/
│   │   ├── server.go          # HTTP server, signal handler
│   │   ├── position.go        # Position manager
│   │   ├── orders.go          # Order placement logic
│   │   ├── risk.go            # Risk limits, kill switch
│   │   ├── exchange.go        # Exchange REST API interface
│   │   ├── binance.go         # Binance order API
│   │   └── paper.go           # Paper trading implementation
│   └── common/
│       ├── tick.go            # Tick type
│       └── config.go          # Config loading helpers
├── configs/
│   ├── poller.yaml.example
│   ├── executor.yaml.example
│   └── ding-trading.yaml.example
├── trade-log/                 # gitignored
└── go.mod
```

This is a separate repo from Ding. Ding stays clean as a general-purpose alerting tool.

## Testing Strategy

### Unit tests (no exchange needed)

- Position manager: open/close positions, enforce limits, calculate P&L
- Risk manager: kill switch triggers at threshold, daily trade caps
- Signal parser: rule names map to correct actions
- Exchange adapters: parse mock WebSocket messages into normalized ticks

### Integration tests (paper mode)

- Full pipeline with mock WebSocket server -> poller -> Ding -> executor in paper mode
- Verify: signal fires -> position opens -> stop-loss/take-profit triggers -> position closes -> P&L logged

### Paper trading soak test

- Run full system against real exchange WebSocket feeds with `mode: paper` for 1-2 weeks minimum
- Analyze trade log: win rate, average P&L per trade, drawdowns, fee impact
- Non-negotiable before switching to `mode: live`

## Profitability Analysis

### Fee structure by exchange

| Exchange | Round-trip cost | Min profitable move |
|---|---|---|
| Binance | ~0.2% | ~0.3-0.4% |
| Coinbase | ~1.0% | ~1.3% |
| Kraken | ~0.4% | ~0.6% |
| Jupiter (Solana DEX) | ~0.25% + <$0.01 gas | ~0.35% |
| Bybit | ~0.2% | ~0.3% |

### Viable strategies at this scale

**Tier 1 (highest probability):**
- Momentum on volatile altcoins (SOL, DOGE, PEPE) — 2-5% intraday moves are routine
- Increased position size with decreased trade frequency — fewer bigger trades beats many small ones when fees are percentage-based

**Tier 2 (works but harder):**
- Mean reversion on majors (BTC, ETH) — buy 2%+ dips below hourly average, sell on recovery
- Multi-timeframe confirmation — only act when 2m and 15m trends agree

**Tier 3 (future roadmap):**
- Volatility regime detection — modulate other strategies based on volatility levels

### Fee minimization

1. Use limit orders (maker fee, sometimes 0%)
2. Hold exchange tokens for fee discounts (BNB gives 25% off on Binance)
3. Volume tiers reduce fees as trading volume grows
4. Batch exits — prefer single take-profit over multiple partial exits

### P&L projections ($1,000 starting capital, $100 position size, 0.2% fees)

| Scenario | Trades/day | Win rate | Avg win | Avg loss | Daily P&L |
|---|---|---|---|---|---|
| Pessimistic | 4 | 40% | 2.0% | -2.0% | -$0.96 |
| Realistic | 4 | 50% | 2.5% | -1.5% | +$1.60 |
| Optimistic | 6 | 55% | 3.0% | -1.5% | +$4.59 |

### Non-negotiable launch sequence

1. Paper trade for 2 weeks minimum
2. Analyze trade log: if win rate < 45% or daily P&L consistently negative, tune rules
3. Go live with $500 (not full $2k)
4. Run 1 week, compare live P&L to paper results
5. If live results significantly worse than paper, investigate before scaling
6. Scale to full capital only after 2+ weeks of live positive results

## Development Workflow

```bash
# Terminal 1: Ding with trading rules
ding -c configs/ding-trading.yaml

# Terminal 2: Market poller
crypto-poller -c configs/poller.yaml

# Terminal 3: Trade executor in paper mode
crypto-executor -c configs/executor.yaml

# Terminal 4: watch activity
tail -f trade-log/2026-04-11.jsonl | jq .
```
