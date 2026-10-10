# A health watch and a deterministic fixture

This example uses HTTP status and a console destination, so it needs no secret.
Replace the URL and ID with the user's intended values. Reuse an existing
destination from `ding_list_destinations` when appropriate.

```yaml
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api-health, name: API health}
spec:
  source: {type: http, url: https://api.example.com/health, every: 5s, timeout: 2s}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {trigger: transition, consecutive: 3, recoverAfter: 2, onUnknown: hold-incident}
  destinations: [{ref: local-console, events: [firing, recovered]}]
---
apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: local-console}
spec: {type: console}
```

Pass a JSONL fixture as `fixture` to `ding_preview_changes`. These observations
should fire once on the third failure, then recover on the second healthy check.
Accepted timestamps drive evaluation. A source timeout is `health: unknown`, not
an invented HTTP 500. Fixture evaluation never performs network requests.

```jsonl
{"sequence":1,"acceptedAt":"2026-10-10T00:00:00Z","health":"ok","fields":{"http.status":500}}
{"sequence":2,"acceptedAt":"2026-10-10T00:00:05Z","health":"ok","fields":{"http.status":500}}
{"sequence":3,"acceptedAt":"2026-10-10T00:00:10Z","health":"ok","fields":{"http.status":500}}
{"sequence":4,"acceptedAt":"2026-10-10T00:00:15Z","health":"ok","fields":{"http.status":200}}
{"sequence":5,"acceptedAt":"2026-10-10T00:00:20Z","health":"ok","fields":{"http.status":200}}
```

For custom HTTP JSON, use source `fields` selectors (and optional `jq`) to extract
the desired scalar fields. Other conditions include `changed`, `new-event`
(requires `dedupFor`), numeric windows, and `missingFor`. Use the daemon's compiler
to validate combinations; do not invent operators or substitute model reasoning
for deterministic condition evaluation.
