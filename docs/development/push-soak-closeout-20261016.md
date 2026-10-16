# October 2026 push soak closeout

The user ended the old SQLite soak during the move to PostgreSQL. This is an
**early stop with failed gates**, not a completed 72-hour pass.

| Measurement | Result |
| --- | --- |
| Run | `push-04da8c7b69a1524a` |
| Application commit | `f8cd4f8b8d55f792f2e3674c68100acec1eea137` |
| Start | October 13, 18:31 UTC |
| Operator stop | October 16, 03:55:29 UTC |
| Duration | 57 hours, 24 minutes |
| Expected scheduled samples | 3,444 |
| Recorded / accepted batches | 3,442 / 3,442 |
| Observations | 61,956 |
| Accepted JSON bytes | 2,082,371 |
| Exact replays verified | 3,442 |
| Changed-byte conflicts correctly rejected | 57 |

A Cloudflare Worker sent one roughly 600-byte JSON document per minute. Each
contained 18 repository groups and their synthetic boolean failure states.
Every accepted batch was replayed with the same idempotency key. Changed bytes
with an existing key were checked hourly. This was a correctness and longevity
exercise, not a load test.

Two scheduled samples were missing on October 15 at 08:26 and 08:27 UTC. One
telemetry check failed on October 14 at 08:00 UTC; its push and application
health check succeeded. Every recorded push was accepted and its replay
returned the original acceptance ID and sequence range. Measurement totals
reconcile exactly with durable usage after subtracting the premeasurement
baseline: no replay added usage or observations.

Final recovery was performed offline after measurement, through the Go runtime,
while the old deployment was in maintenance. All 18 groups recovered. The test
watch was paused and this rerun's producer and inspector grants were revoked.
Both old soak observers' Cron schedules and follow-up automations were disabled;
the push Worker's producer and inspector secrets were removed. The evidence,
older failed attempt, and shared Railway project token were retained.

Private evidence includes the 24-hour, 48-hour and final runtime checkpoints,
full scheduled samples, receipt reconciliation, and cleanup results. The old
SQLite data was encrypted, downloaded and restored in quarantine; all three
databases passed integrity checks. Keys are kept separately from the archive.

Multi-user load, external notification delivery and a full 72-hour uninterrupted
window remain unqualified by this run. Offline final recovery does not establish
end-to-end recovery through the external producer. A future soak needs a new
explicit start and a manifest pinned to the application under test.
