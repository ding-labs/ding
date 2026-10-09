# Watch manifest reference

Manifests use `apiVersion: ding.ing/v1alpha1` and `kind: Watch` or `Destination`.
Each document has a stable `metadata.id` and an optional `metadata.name`.
Unknown fields fail validation. The Go compiler owns validity.

[Download the generated JSON Schema](../assets/watch-v1alpha1.json). Use it for
editor assistance with the same runtime version; it describes definitions, not
the control API. `ding explain FILE --json` shows normalized defaults, revision,
execution fingerprint, and permissions.

| Watch field | Purpose |
| --- | --- |
| `source` | `http`, `command`, or `push`, with source-specific connection/execution details |
| `source.jq`, `source.fields` | Bounded projection followed by selected scalar fields |
| `source.observedAtField` | Provider timestamp metadata; conditions still use accepted time |
| `condition` | Typed comparison, numeric expression, `missingFor`, `changed`, or `new-event` |
| `policy` | Trigger, consecutive/recovery counts, unknown handling, and interval/gap behavior |
| `groupBy`, `match` | Entity identity and input selection |
| `message` | Bounded message template |
| `destinations` | Destination references and selected event types |
| `limits` | Entity, sample, byte, output, and idle-retention budgets |

Typed operators are `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `changed`, and `new-event`.
Ordered comparisons require numeric values. `missingFor` and `numeric` are separate
condition forms. See the [full watch contract](../development/watch-contract.md)
for limits and defaults, and [evaluation semantics](../development/evaluation-contract.md)
for windows and unknown input.

Destination types are `console`, `webhook`, `slack`, and `discord`. Network
endpoints use `urlRef: {env: NAME}`; headers also use secret references. Retry policy
fields include `maxAttempts`, `maxAge`, and `initialBackoff`. [Notification guide](../guides/notifications.md).
