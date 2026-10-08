# Watch authoring contract (experimental)

P06 introduces the temporary `ding-watch` binary. The legacy `ding` binary is
unchanged. The commands currently available are `validate`, `explain`, and
`--version`. Runtime commands are added at their subsequent implementation gates.

```sh
go run ./cmd/ding-watch validate examples/watches/api-health.yaml --json
go run ./cmd/ding-watch explain examples/watches/api-health.yaml
```

A manifest holds one or more YAML/JSON resources separated by `---`: `Watch` and
`Destination`, both at `apiVersion: ding.ing/v1alpha1`. IDs are stable identifiers,
1–128 ASCII letters, digits, dots, underscores, or hyphens, starting with a letter
or digit. Display names do not provide identity. Unknown fields, duplicate
resource IDs/fields, YAML aliases, unsupported types, and invalid combinations
fail compilation. The manifest limit is 1 MiB and 100 resources.

[The example](../../examples/watches/api-health.yaml) declares an HTTP check every
five seconds, triggers after three actual HTTP 5xx observations, and recovers
after two healthy observations. A transport timeout is unknown/source health,
not a fabricated HTTP 500. `urlRef.env` identifies a secret; compiling, validating,
and explaining do not read the variable. Destinations must resolve when applied.

Source contracts are `http`, `command`, and `push`. HTTP permits one literal URL
or URL secret reference, secret header references, and selected JSON field paths.
Command definitions require explicit argv and an absolute working directory;
they are trusted local execution, not a sandbox. Environment variables must be
explicitly referenced. Push has no polling fields. Polling defaults to every
five seconds with a two-second timeout. No source I/O runs during compilation.
These source shapes describe the alpha contract; runtime adapter availability is
introduced in P09 and P11, not implied by `validate` succeeding in P06.

Conditions support typed scalar comparisons (`eq`, `ne`, `gt`, `gte`, `lt`,
`lte`), the existing numeric expression syntax via `numeric` plus `field`,
`missingFor`, `changed`, and provider-ID-based `new-event` with `dedupFor`.
Comparison values are explicit, including `null`; absent observation fields are
unknown, not null. Numbers use Go/JSON binary64 semantics; opaque provider IDs
should be strings. Run-lifetime numeric aggregation is rejected. Numeric windows
must be positive and at most 30 days. Missing-data and change/event evaluation
arrive with their runtime milestones. The compiler never invents a provider or
approximates an unsupported condition.

Policies default to `transition`, one match, one recovery, and
`onUnknown: hold-incident`. `level` requires an explicit interval; `0s` means
one eligible event per accepted match. Change/new-event conditions require level
policy with one consecutive match. Polling freshness defaults to twice the poll
interval. Grouping uses only explicitly declared `groupBy` fields, preserving
scalar types and distinguishing absent, null, and empty values.

Compilation normalizes defaults and produces a SHA-256 revision of the entire
definition plus a state fingerprint. Display name, message, and destination-only
changes preserve the fingerprint. Source interpretation, grouping, matching,
condition, temporal policy, and resource-bound changes reset it. Compilation
copies its inputs; caller maps/slices are not mutated. File edits never imply a
runtime application.

Default per-watch bounds are 1,000 entities, 10,000 window samples, 1 MiB response
and transformed output, 100 transformed observations, and 24-hour idle retention.
The runtime must reject/degrade overflow instead of silently truncating a window.
Open incidents and active windows have additional retention protections. Generic
webhook, console, Slack, and Discord are the destination contract. Default retry
policy is eight attempts over 24 hours, starting with a one-second backoff.
Already committed deliveries will retain their destination revision.

The [JSON Schema](../../schemas/watch-v1alpha1.json) describes structural fields
and selected constraints. The compiler is authoritative for semantic checks and
capabilities. Regenerate with `go run ./cmd/watch-schema`; a test checks that the
artifact matches the Go types. Source examples and normalized output are also
validated with an independent Draft 2020-12 validator during this milestone.

`--json` writes a single success envelope to stdout or error envelope to stderr,
with a nonzero exit code on error. Stable error codes currently include
`read_failed`, `invalid_manifest`, and `invalid_arguments`:

```json
{"apiVersion":"ding.ing/v1alpha1","error":{"code":"invalid_manifest","message":"kind must be Watch or Destination"}}
```

Success data contains compiled watches and destinations, normalized definitions,
revisions, fingerprints, and required permissions. No resolved credentials appear
in definitions or output. New runtime commands reuse this envelope.
