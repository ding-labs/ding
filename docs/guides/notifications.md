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
