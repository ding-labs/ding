# Recognize changes and new events

## Watch a version change

Save this as `version.yaml`. Push `{"version":"1.0"}` and then
`{"version":"1.1"}` to `/v1/ingest/deployed-version` with fresh idempotency keys.
The first input establishes a baseline; the second records a `changed` event.

```yaml
{{ snippet:examples/watches/version-change.yaml }}
```

[Download the manifest](../assets/examples/version-change.yaml). Validate and
apply it through the [normal review flow](../configuration.md). Baselines preserve
types: the string `"1"` differs from the number `1`. Missing fields are unknown;
null is a value. Baselines survive restart and unknown observations.

## Deduplicate provider events

```yaml
{{ snippet:examples/watches/provider-events.yaml }}
```

[Download the provider-event manifest](../assets/examples/provider-events.yaml).
The provider must supply stable string IDs. This example accepts an `events` array,
selects fields, and emits each new ID once within a 24-hour deduplication horizon.
Repeated sightings do not extend that horizon. After expiry an ID may emit again.

Both `changed` and `new-event` use level policy with an explicit interval; they do
not open recovery incidents. Resource exhaustion becomes unknown rather than
silently evicting still-valid state. Provider access and upstream calculations
remain your responsibility.
