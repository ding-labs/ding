# Understand a watch

A **watch** declares a source, selected fields, a condition, an emission policy,
and destination references. A **destination** declares how notifications are
sent. Stable IDs let tools inspect and update the same objects.

## From input to notification

1. A source produces an **observation**, including selected fields and health.
2. A condition evaluates it against retained state.
3. An **event** records a decision or lifecycle change and its definition revision.
4. A **delivery intent** retains the event's payload and destination revision.
   Workers record attempts and retry eligible failures.

Observations, state, events, and delivery intents commit together in SQLite.
A failed transaction cannot advance the source while losing the event.

## Read each state separately

| Dimension | What it tells you |
| --- | --- |
| Lifecycle | Running, paused, or deleted: whether acquisition is enabled. |
| Condition | Waiting, matching, open incident, recovering, or unknown. Change/event policies use their own event vocabulary. |
| Source | Whether data was acquired and interpreted successfully. An HTTP 503 can be a successful acquisition that satisfies a failure condition. |
| Delivery | Queued, sending, retrying, delivered, permanent failure, exhausted, or canceled. |
| Browser connection | When the planned Console last read data; disconnecting does not stop a daemon. |

Unknown input preserves an open incident and resets incomplete continuity evidence.
An old push input is not automatically an error without a declared freshness rule.
A grouped watch can have different states for different entities.

## Time and evidence

Conditions use accepted observation time. Provider time can be retained as
metadata but does not drive windows. Windows include `(accepted time - window,
accepted time]`; their exact lower boundary is excluded.

Events retain the relevant definition and replay checkpoint. A checkpoint can
explain a prior count without retaining every original request. Retention gaps,
unavailable evidence, and unsupported replay are explicit. Never infer a full
historical trace from one event. [Investigate an event](../guides/investigate.md).

## Changing a definition

Validate, explain, and dry-run before applying. Compatible message or destination
edits preserve state. Changes to interpretation or conditions reset it with a
`state_reset` event. Expected revisions detect concurrent edits. Already queued
deliveries keep their original destination revision.

[Manifest reference](../reference/manifest.md) · [Lifecycle contract](../development/lifecycle-contract.md)
