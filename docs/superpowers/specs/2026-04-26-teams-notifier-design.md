# Microsoft Teams Notifier — Design Spec

**Date:** 2026-04-26
**Status:** Approved

## Overview

Add a Microsoft Teams notification channel to DING targeting the current Microsoft-recommended integration: Power Automate / Workflows incoming webhooks with Adaptive Cards. The implementation follows the established notifier pattern (Slack, Discord, Telegram, PagerDuty) exactly, with Teams-specific payload formatting.

## Architecture

Five touch points:

| File | Change |
|------|--------|
| `internal/notifier/teams.go` | New — `TeamsNotifier` struct, methods, payload builder |
| `internal/notifier/teams_test.go` | New — unit + integration tests |
| `internal/config/config.go` | Add `"teams"` case to `Validate()` |
| `internal/server/server.go` | Add `"teams"` case to `buildFromConfig()` |
| `ding.yaml.example` | Add Teams example block |

No new config fields. The existing `NotifierConfig` struct (`type`, `url`, `max_attempts`, `initial_backoff`) covers everything Teams needs.

## Config Surface

```yaml
notifiers:
  alert-teams:
    type: teams
    url: https://prod-XX.westus.logic.azure.com:443/workflows/...
    max_attempts: 3
    initial_backoff: 1s
```

`Validate()` enforces that `url` is non-empty for type `teams`, sets `max_attempts` default of 3 and `initial_backoff` default of 1s — identical to the `slack` and `discord` cases.

## TeamsNotifier Struct

Identical structure to `SlackNotifier` and `DiscordNotifier`:

```go
type TeamsNotifier struct {
    webhookURL     string
    client         *http.Client
    maxAttempts    int
    initialBackoff time.Duration
    queue          chan retryItem
    stop           chan struct{}
    stopOnce       sync.Once
    collector      *metrics.Collector // may be nil
}
```

Constructor signature matches Slack/Discord:

```go
func NewTeamsNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TeamsNotifier
```

Methods: `Send(alert evaluator.Alert) error`, `Stop()`, `Drain(timeout time.Duration)`, plus unexported `worker()` and `deliver(item retryItem) error`.

## Payload Structure

Teams Workflows webhooks require a message envelope wrapping the Adaptive Card:

```
teamsMessage
  type: "message"
  attachments[]
    contentType: "application/vnd.microsoft.card.adaptive"
    contentUrl:  null
    content: teamsCard
      $schema:  "http://adaptivecards.io/schemas/adaptive-card.json"
      type:     "AdaptiveCard"
      version:  "1.5"
      body: []interface{}
        teamsTextBlock  — alert.Rule (Weight: "Bolder", Size: "Medium")
        teamsTextBlock  — alert.Message, omitted when empty (Wrap: true)
        teamsFactSet    — structured fields (see below)
        teamsTextBlock  — "Fired at <RFC3339>" (IsSubtle: true, Size: "Small")
```

Go types:

```go
type teamsTextBlock struct {
    Type     string `json:"type"`
    Text     string `json:"text"`
    Weight   string `json:"weight,omitempty"`
    Size     string `json:"size,omitempty"`
    Wrap     bool   `json:"wrap,omitempty"`
    IsSubtle bool   `json:"isSubtle,omitempty"`
}

type teamsFact struct {
    Title string `json:"title"`
    Value string `json:"value"`
}

type teamsFactSet struct {
    Type  string      `json:"type"`
    Facts []teamsFact `json:"facts"`
}

type teamsCard struct {
    Schema  string        `json:"$schema"`
    Type    string        `json:"type"`
    Version string        `json:"version"`
    Body    []interface{} `json:"body"`
}

type teamsAttachment struct {
    ContentType string     `json:"contentType"`
    ContentURL  *string    `json:"contentUrl"`
    Content     teamsCard  `json:"content"`
}

type teamsMessage struct {
    Type        string            `json:"type"`
    Attachments []teamsAttachment `json:"attachments"`
}
```

`ContentURL *string` serializes as JSON `null` when nil (required by Teams).

## FactSet Field Ordering

Mirrors Slack and Discord exactly:

1. Metric (`alert.Metric`)
2. Value (`alert.Value`, `%g` format)
3. exit code (`alert.Floats["exit_code"]`) — omitted if absent
4. duration (`alert.Floats["duration_seconds"]`, `%.1fs` format) — omitted if absent
5. Run-context labels in stable order: `branch`, `commit`, `repo`, `workflow`, `job`, `actor`, `runner`, `run_id` — omitted when empty; `commit` truncated to 7 chars

No field limit (Teams FactSet supports unlimited facts; Slack caps at 10).

## HTTP Delivery

- POST to `webhookURL` with `Content-Type: application/json`
- 2xx/3xx → success
- 4xx → logged, not retried (client error)
- 5xx or connection error → retried with exponential backoff (`initialBackoff * 2^(attempt-1)`)
- Drops after `maxAttempts` exhausted

## Testing

Tests in `internal/notifier/teams_test.go` using `httptest.NewServer`:

**`buildTeamsPayload` unit tests:**
- Rule name as bold TextBlock header
- Message TextBlock present when non-empty, absent when empty
- Metric and value always first two FactSet facts
- `exit_code` and `duration_seconds` from `alert.Floats` when present
- Run-context labels when present; commit truncated to 7 chars
- Fired-at footer present

**`TeamsNotifier` integration tests:**
- 2xx → `collector.IncrWebhookSuccess()`
- 5xx → retry with backoff → drop after `maxAttempts` → `collector.IncrWebhookFailed()`
- 4xx → logged, not retried
- Queue full → drop → `collector.IncrWebhookDrop()`
- `Drain` → flushes queue before stopping

Scope and coverage identical to `slack_test.go` and `discord_test.go`.
