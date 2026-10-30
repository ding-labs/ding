# Send notifications and inspect delivery

The watch preview supports console output, webhook, Slack incoming webhook, and
Discord webhook destinations. Store credential-bearing URLs in environment
variables on the daemon host and reference their names in YAML.

```yaml
apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: ops}
spec:
  type: slack
  urlRef: {env: OPS_SLACK_WEBHOOK_URL}
```

Use `type: webhook` or `type: discord` for those endpoints. Add the destination and
reference to a multi-document manifest, then validate and dry-run the entire file.
For a transition watch, an explicit reference can select firing and recovery:

```yaml
destinations:
  - ref: ops
    events: [firing, recovered]
```

Set the environment variable before starting the daemon. `ding doctor --json`
reports whether referenced credentials are present without returning their values.
A missing variable causes inspectable delivery failure.

## Status bells

Slack messages and Discord embeds include a Ding bell and a text status label:

| Status | Bell | Events |
| --- | --- | --- |
| Warning | Yellow | Sampling gaps, state resets, or explicit warning severity |
| Error | Red | Firing incidents, source errors, or explicit error severity |
| Recovered | Green | `recovered` and `source_recovered` |

For warning watches and log events, include a string `severity`, `level`, or
`log.level` in the projected observation fields, in that priority order. Values
`warn` and `warning` select yellow; `error`, `err`, `fatal`, `critical`, `crit`, and
`panic` select red. Matching ignores case and surrounding whitespace. A firing
event defaults to red when no recognized severity is present. Recovery events
always use green even if their retained fields still describe an earlier error;
source errors always use red, and gaps/state resets always use yellow. These
fields affect notification presentation, not which events a watch emits or routes.

Changes, new events, tests, and administrative messages without a recognized
severity have no status bell. They do not establish that a service is healthy.
`source_recovered` means observations resumed; a condition incident may still
be open. Include `source_error`, `source_recovered`, `gap`, or `state_reset` in a
destination's event selection to receive those notifications.

Slack uses an image inside the message because incoming webhooks retain the
[Slack app's configured avatar](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/).
Discord uses a thumbnail and matching embed border. Both fetch the shared PNGs
from `https://ding.ing/assets/notifications/`; publish the website assets before
deploying a runtime that uses them. Text labels remain available if images are
blocked. Generic webhooks and console JSON retain the original event envelope.
Queued notifications and manual retries retain the payload pinned at creation.

Desktop notifications carry the same status and text label. Linux notifications
send the embedded bell through the standard `image-data` hint, without fetching
an image from the network. The desktop notification server controls its display.
macOS adds the matching bell as a bundled notification attachment, which the OS
may show in the expanded alert. Its app icon remains the Ding brand mark.

## Follow an event to its delivery

```sh
./ding watch inspect latency --state-dir ./ding-state --json
./ding delivery inspect DELIVERY_ID --state-dir ./ding-state --json
```

Replace `DELIVERY_ID` with an ID returned by inspection. Read the pinned destination
revision, attempt results, and any scheduled retry. Acquisition and receiver errors
are different: an HTTP source's 503 does not describe a notification endpoint.

Ding uses bounded retries and durable backoff. Remote receipt is at least once;
webhook receivers should deduplicate stable event IDs. Webhooks include
`Idempotency-Key` and `X-Ding-Event-Id` headers.

## Retry a terminal failure

```sh
./ding delivery retry DELIVERY_ID --state-dir ./ding-state --json
```

Only permanent, exhausted, or canceled deliveries are eligible. This can resend a
notification already accepted remotely. It retains the original payload and
destination revision; editing today's destination does not rewrite queued work.
Pending, sending, and delivered jobs cannot be manually retried. If a request's
outcome is uncertain, inspect the intent before submitting again.
