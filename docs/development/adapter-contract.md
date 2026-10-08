# Initial adapter contracts

HTTP and command sources implement the same acquisition interface. Push input
uses the same projection, evaluator, transaction, quotas, and outbox, with a
separate authenticated entry point. No source owns an evaluator or delivery queue.

## Projection and provider time

`source.jq` runs first. `source.fields` then selects scalar values from dotted
paths in each output. Without field selection, command/push/jq output must be a
flat object containing strings, finite numbers, booleans, or null. HTTP sources
always add the real `http.status`; a projection cannot overwrite that field.
HTTP without fields or jq retains only status, without parsing the response body.
A missing selected field stays absent. Invalid JSON, UTF-8, object shape, or
selected field types make an acquired poll unknown; invalid push requests are
rejected without an acceptance receipt.

Projection checks input bytes, total serialized output bytes, and output count,
and uses a 100 ms cancellation context. jq has no environment, filesystem module,
network, or command loader. Cancellation is cooperative inside gojq; these limits
do not constitute an OS memory sandbox for arbitrary authored programs. An empty
jq result produces an unchanged/freshness observation and no condition sample.

`observedAtField` optionally names a selected field containing an RFC3339 time.
It becomes observation metadata and never replaces accepted time for conditions
or windows. The generated JSON Schema includes this optional source field.

## Commands

A command declares an explicit argv and absolute working directory. Program
lookup for a non-absolute argv[0] uses the daemon's PATH; using an absolute program
path makes that choice explicit. The child receives only configured environment
references (plus Windows SystemRoot, supplied by Go). No shell is inserted. The
command is trusted local configuration, runs with the daemon user's permissions,
and should be read-only or safely repeatable.

Stdout must contain one JSON document. Stdout and stderr share the configured
output budget; only projected stdout is retained. Nonzero exit, timeout, excess
output, or malformed JSON becomes an unknown observation with a redacted code.
The command's timeout includes process execution and projection. Unix processes
run in their own process group, which is killed on timeout and after completion.
Windows starts the process suspended, assigns a kill-on-close Job Object, and
then resumes it. Descendants cannot race job assignment. This follows Microsoft's
[creation flag](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags)
and [Job Object](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
contracts. These are lifecycle controls, not a security sandbox against a
malicious command deliberately escaping its process tree.

## Push

`POST /v1/ingest/{watch-id}` accepts the source JSON document using the ingest
bearer token. The admin token is not accepted for ingest, and the ingest token
cannot manage watches. Obtain the ingest token from the private state directory's
`tokens.json`; keep it in a secret store rather than a watch manifest. Polls,
commands, and push parsing share the 32-slot acquisition budget. Admission occurs
before reading the bounded request body.

A successful response is HTTP 202 with a versioned receipt containing the first
and last accepted observation sequence. A supplied `Idempotency-Key` (up to 256
bytes) identifies the request within the active watch generation for 24 hours.
Concurrent retries commit once and return the original receipt. Without a key,
each request is a new input. Validate and project the entire batch before entering
the transaction; any transactional failure rolls back all its outputs.

Invalid input is 400, excessive input size 413, quota pressure 429, an inactive or
replaced source 409, and unavailable workers/store 503. Only a committed response
is an acceptance acknowledgment. A client that loses the response can retry with
the same key. `examples/watches/provider-events.yaml` demonstrates a batch of
provider events and explicit provider timestamps.

## Change and new-event conditions

`changed` establishes a typed baseline on the first valid observation, emits on
subsequent changes, and records old/new observation evidence. Null is a value;
a missing field is unknown. Baselines survive unknown observations and restart.
`new-event` requires a nonempty string ID of at most 256 bytes and a declared
`dedupFor` horizon. Duplicate sightings do not extend the first sighting's horizon.
Expired IDs may emit again. `limits.maxSamples` also caps retained provider IDs;
exhaustion is unknown, with no silent eviction of still-valid deduplication state.
Both policies use `level` with an explicit interval; `0s` emits each qualifying
change/new event. They do not open recovery incidents. Compatible baseline and
unexpired dedup state remain pinned through idle retention.

## Slack and Discord

Provider renderers create immutable outbox payloads. They share webhook leases,
retry policy, provider backoff, and acknowledgment classification. Slack uses
plain-text blocks, bounded descriptions, and explicit event IDs; only its `ok`
response is success. Discord uses bounded embeds and disables allowed mentions;
a successful webhook HTTP response is accepted. Formatting and delivery follow
[Slack incoming webhooks](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/)
and [Discord webhook](https://docs.discord.com/developers/resources/webhook)
contracts. Truncation preserves valid UTF-8; full event evidence remains in Ding.
