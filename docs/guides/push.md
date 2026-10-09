# Receive JSON from your software

Start with the [first-watch tutorial](first-watch.md). Send JSON to
`POST /v1/ingest/{watchId}` with the ingest bearer credential. Select the fields
the condition needs; selected data becomes retained evidence.

A 202 response acknowledges durable acceptance, not delivery of a notification.
Retry an uncertain request with the same idempotency key within its receipt
horizon. Keys are scoped to the active watch generation and retained for 24 hours;
after expiry the same key can represent a new input.

| Response | Producer action |
| --- | --- |
| 202 | Persist the receipt; the input committed. |
| 400 or 413 | Correct invalid or oversized input before retrying. |
| 401 | Check the ingest token and target daemon. |
| 409 | Inspect whether the watch is inactive or replaced. |
| 429 | Respect pressure and back off. |
| 503 | Retry with bounded backoff and the same idempotency key. |

Use `source.jq` for a bounded projection, then `source.fields` to select scalars.
A batch commits all projected outputs or none. Never send arbitrary remote input
to a command source. See [provider events](changes.md) and the [API](../api.md).
