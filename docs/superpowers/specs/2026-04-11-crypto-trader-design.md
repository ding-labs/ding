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
2. Normalizes each tick to Ding's JSON format: `{"metric": "price", "value": 67432.50, "pair": "BTC/USDT", "exchange": "binance", "bid": 67432.00, "ask": 67433.00}`
3. POSTs to Ding's `/ingest` endpoint
4. Ding's JQ transform extracts metric/value; labels (pair, exchange) pass through for rule matching
5. Rules evaluate (e.g., `avg(value) over 2m > avg(value) over 10m * 1.005` detects upward momentum)
6. When a rule fires, Ding sends a webhook to the Trade Executor
7. Executor interprets the signal (rule name encodes intent), checks risk limits, places an order

**Key property:** Ding doesn't know it's trading. It evaluates rules on streaming numeric data and fires webhooks. Trading semantics live in rule names and the executor's interpretation.

## Component 1: Market Poller

A small Go service that turns exchange WebSocket feeds into Ding-compatible JSON.

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
```

### Design

- Connects to each exchange's public WebSocket API (no API keys needed)
- Subscribes to trade/ticker streams for configured pairs
- Normalizes exchange-specific JSON via per-exchange adapters
- Batches ticks and POSTs to Ding on `flush_interval` (500ms default)
- Each exchange gets its own goroutine with reconnection logic

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
- No strategy logic
- No state beyond current WebSocket connection
- No API key management

## Component 2: Ding Trading Rules

No code changes to Ding. Trading strategies are expressed as YAML rules.

### Example Configuration

```yaml
server:
  port: 8080
  format: json
  jq: '{metric: .metric, value: .value, pair: .pair, exchange: .exchange, volume_24h: .volume_24h}'

notifiers:
  - name: executor
    type: webhook
    url: http://localhost:9090/signal
    max_retries: 1

rules:
  # Momentum strategies
  - name: momentum_buy
    match:
      pair: BTC/USDT
    condition: "avg(value) over 2m > avg(value) over 10m * 1.005"
    cooldown: 5m
    message: "BTC short-term avg crossed above long-term by 0.5%"
    alert: [executor]

  - name: momentum_sell
    match:
      pair: BTC/USDT
    condition: "avg(value) over 2m < avg(value) over 10m * 0.995"
    cooldown: 5m
    message: "BTC short-term avg dropped below long-term by 0.5%"
    alert: [executor]

  # Mean reversion
  - name: mean_reversion_buy
    match:
      pair: ETH/USDT
    condition: "value < avg(value) over 1h * 0.98"
    cooldown: 15m
    message: "ETH dropped 2% below 1h average"
    alert: [executor]

  - name: mean_reversion_sell
    match:
      pair: ETH/USDT
    condition: "value > avg(value) over 1h * 1.02"
    cooldown: 15m
    message: "ETH rose 2% above 1h average"
    alert: [executor]

  # Volatility spike detection
  - name: volatility_alert
    match:
      pair: SOL/USDT
    condition: "max(value) over 5m - min(value) over 5m > avg(value) over 5m * 0.03"
    cooldown: 10m
    message: "SOL 5m range exceeds 3% of average"
    alert: [executor]
```

### Design choices

- **Rule names encode intent.** Executor parses rule names: `*_buy` -> buy, `*_sell` -> sell, anything else -> log only.
- **Cooldowns prevent overtrading.** First line of defense against fee burn.
- **Thresholds include fee buffer.** The `* 1.005` multiplier means "only signal when the move is at least 0.5%," clearing the ~0.2% round-trip fee.
- **`max_retries: 1`** on the notifier. Stale trading signals are worse than missed ones.
- **New strategies** are added by writing a new rule block and hitting `/reload`. No restarts, no code changes.

### Limitation

Cross-pair correlations (e.g., "BTC moves but ETH doesn't follow") cannot be expressed as a single Ding rule. This would require future Ding enhancement or executor-side logic. Not needed for v1.

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

Background goroutine checks all open positions every second:

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
│   │   └── bybit.go           # Bybit WebSocket adapter
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
