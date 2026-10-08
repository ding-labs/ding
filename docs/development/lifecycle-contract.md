# Lifecycle, timers, and resource bounds

Applying a new revision is atomic with its state decision. The execution
fingerprint includes source interpretation, grouping, condition, temporal policy,
and limits. Display-name/message/destination-only changes preserve compatible
state. Incompatible changes record `state_reset`, clear state and timers, and
start the new definition. Every replacement increments a generation and cancels
older acquisition. Late results receive `stale generation` and cannot change the
new state. Destination revisions already attached to outbox jobs remain pinned.
An expected revision rejects concurrent editing conflicts. Dry-run computes this
decision without writing anything.

```sh
ding-watch watch pause api-health --state-dir ./ding-state --json
ding-watch watch resume api-health --state-dir ./ding-state --json
ding-watch watch delete api-health --state-dir ./ding-state --cancel-pending --json
```

Pause cancels acquisition and removes deadlines while preserving incident state.
Committed delivery continues. Resume records replayable gap observations, clears
incomplete continuity, and restarts acquisition and missing-data deadlines from
resume time. It does not invent samples for the pause. Delete creates a tombstone
and preserves history; pending jobs continue unless `--cancel-pending` is given.
Explicit cancellation invalidates leases, but cannot retract a request already
accepted remotely. A retained tombstone cannot be silently resurrected; use a new
ID. Once all its evidence expires, retention can remove the tombstone too.

`missingFor` uses persisted per-entity deadlines. It requires transition policy
with consecutive=1 and recoverAfter=1: repeated timer ticks are not independent
observations. The deadline fires once until a fresh observation recovers it.
Successful HTTP 304 is freshness for this condition. Unknown transport/data does
not postpone the deadline, and a timer does not mark a failed source healthy.
An ungrouped watch starts its deadline when applied; grouped watches start each
entity's deadline when first observed. Ding cannot infer never-seen group IDs.
Timer observations retain their target entity and deadline for replay. Their
transaction removes the deadline, saves incident state, records evidence, and
enqueues delivery; they never advance the source poll checkpoint.

Default process limits are 1,000 active watches, 10,000 pending/leased deliveries,
and 1 GiB of live SQLite pages. Defaults for each watch remain 1,000 entities,
10,000 exact-window samples, 1 MiB inputs, and 100 outputs. The daemon exposes
`--max-watches`, `--max-pending`, `--max-store-bytes`, and `--history` for explicit
capacity configuration. SQLite also receives a hard page-allocation limit. WAL
files and backups consume additional filesystem space and must be included in
operator disk provisioning; the page budget is not a filesystem partition cap.

Quota failure rolls back input acceptance and preserves the source checkpoint.
A redacted diagnostic remains on the watch when the store can still write.
Global pressure stops acquisition until delivery or retention frees capacity.
After a failed acceptance, the next accepted batch includes an explicit gap.
Closed idle entities may expire when their window and cooldown no longer need
them. Expiration is recorded as an event. Open incidents and unknown source
health remain visible, even beyond idleTTL, rather than silently disappearing.
This can hold a quota until the source recovers or the operator deletes the watch.

Maintenance runs at startup and every minute. Ordinary events and observations
expire after seven days. Input receipts expire after 24 hours, after which an
idempotency key may represent a new acceptance. Active windows, condition streaks,
current incident/health events, pending deliveries, and all their evidence remain
pinned. Terminal outbox history and unreferenced old revisions can expire. This
allows evidence to outlive the nominal history horizon, within the store budget.

The tests include revision replacement, canceled acquisition, concurrent
pause/inspection, rollback under quota, replayable gap recovery, grouped source
bootstrap, timer restart and stale-deadline fencing, and retention of active
window/incident/outbox evidence.
