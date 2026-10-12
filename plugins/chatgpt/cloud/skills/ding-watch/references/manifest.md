# A cloud health watch

Use the user's public HTTPS URL and an existing destination ID returned by
`ding_list_destinations`. This example references an already configured destination;
it does not create one or accept a notification credential. Respect the current
capabilities if they require a longer polling interval or smaller limits.

```yaml
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api-health, name: API health}
spec:
  source: {type: http, url: https://api.example.com/health, every: 5m, timeout: 5s}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {trigger: transition, consecutive: 3, recoverAfter: 2, onUnknown: hold-incident}
  limits: {maxBytes: 65536, maxEntities: 10, maxSamples: 100, maxOutputs: 10}
  destinations: [{ref: existing-alerts, events: [firing, recovered]}]
```

Preview validates the manifest without polling or delivering a notification.
A small JSONL `fixture` can exercise the condition deterministically. A source
timeout is `health: unknown`, not an invented HTTP status. Use the returned
compiler errors to correct unsupported combinations; do not invent operators.
