# Watch configuration

Build the watch preview using [installation](install.md). A manifest is one or more
YAML documents with `apiVersion: ding.ing/v1alpha1` and `kind: Watch` or
`kind: Destination`. IDs are stable. Unknown fields fail validation.

```yaml
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: latency}
spec:
  source: {type: push}
  condition: {field: latency_ms, operator: gt, value: 300}
  policy: {trigger: transition, consecutive: 1, recoverAfter: 1}
```

Run `ding validate FILE --json` and `ding explain FILE --json` without credentials
or network access. Apply through the running daemon with `ding apply FILE
--dry-run --json`, then `ding apply FILE`. Compatible message/destination edits
preserve state. Changed interpretation or conditions reset state with an event.
Use `--watch ID --expected-revision REVISION` to detect concurrent edits.

Sources: HTTP status and selected JSON fields; explicit command argv returning
JSON; authenticated push JSON. Optional jq projection precedes scalar selection.
Commands need an absolute directory and environment references. Destinations:
console, webhook, Slack incoming webhook, Discord webhook. URLs and headers with
credentials belong in `urlRef: {env: NAME}` or header references. Selected source
fields are retained; choose them deliberately.

Conditions include typed comparisons, numeric aggregates, missing data, value
changes and provider-ID events. Time windows use accepted observation time and
exclude their exact lower boundary. Transition mode fires once until recovery;
level mode requires an explicit interval. Unknown input preserves an open incident
and resets incomplete continuity evidence. A source error is separate from a
condition firing. Missing-data timers can fire without a new observation.

Read the full [authoring contract](development/watch-contract.md),
[evaluation semantics](development/evaluation-contract.md),
[adapter contract](development/adapter-contract.md), and
[lifecycle limits](development/lifecycle-contract.md). For old `rules:` and
`notifiers:` configurations, use [migration](legacy.md).
