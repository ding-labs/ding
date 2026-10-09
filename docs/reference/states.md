# State and event glossary

| Term | Meaning |
| --- | --- |
| `firing` | A condition emitted a firing event according to its policy. |
| `recovered` | A transition incident met its recovery condition. |
| `changed` | A typed value differed from its saved baseline. |
| `new-event` | A provider ID was new within the declared deduplication horizon. |
| `source_error`, `source_recovered` | Source/data health changed. |
| `gap` | Continuity is incomplete; do not infer missing observations. |
| `state_reset` | A definition change reset incompatible state. |
| `paused`, `resumed`, `deleted`, `expired` | A lifecycle or retained-state transition. |

Delivery storage uses `pending` for queued work or scheduled retries, `leased`
for an active attempt, `delivered` for acknowledged success, `permanent` for a
nonretryable rejection, `exhausted` for the end of a bounded policy cycle, and
`canceled` for explicit cancellation. Manual retry is limited to terminal failures
or canceled work and can duplicate a previously accepted notification.

Replay reports whether retained evidence verifies, differs, or is unavailable;
unsupported or malformed evidence is never presented as verified. Read full
[inspection semantics](../development/inspection-contract.md).
