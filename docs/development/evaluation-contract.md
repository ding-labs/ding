# Deterministic evaluation

The condition package receives a compiled definition, prior entity state, an
observation with a positive sequence, and its recorded accepted time. It does
not read wall time or environment variables and does not perform I/O. Evaluation
returns new state and events; the caller must persist them atomically before
using the result. Sequences increase globally in replay and within each entity
in evaluation. Provider observed time is evidence only.

Transition policy emits once when the matching streak reaches `consecutive`.
An open incident closes after `recoverAfter` false observations. Unknown input
clears pending streaks, holds an open incident, and emits source health changes.
Type errors and exhausted exact-window budgets count as unknown data health.
Explicit or excessive sampling gaps clear unproven streaks and emit a gap.
A backward clock is unknown until the accepted-time watermark is reached again.
HTTP 304 advances freshness without counting as a new condition sample, and
without claiming that previously unknown data has become healthy.

Level policy emits matching observations subject to its explicit interval;
`0s` permits one event per matching observation. Entity keys contain only the
declared grouping fields. An unknown observation for an existing entity retains
that identity even when no fields are available. Exact windows include samples
in `(acceptedAt - window, acceptedAt]`. A full sample budget returns unknown;
omitted samples contaminate the window until they expire. No approximate
aggregate is presented as exact. Event evidence includes streak observations and
retained window samples; event IDs derive from revision, entity, type, and
observation sequence. Message rendering is bounded while writing.

Replay uses the same evaluator and recorded logical times:

```sh
go run ./cmd/ding test examples/watches/api-health.yaml \
  --events testdata/watches/api-health.jsonl --json
```

The fixture produces one firing and one recovery. JSONL records carry explicit
sequence, acceptedAt, health, and typed fields. Replay never acquires live data,
resolves secrets, or sends notifications. Records are bounded by the manifest's
byte limit; total input is limited to 100,000 observations. Missing-data timers are replayable records with explicit deadlines and entities.
Change/new-event conditions retain typed baselines and bounded provider-ID horizons;
see [the adapter contract](adapter-contract.md).
