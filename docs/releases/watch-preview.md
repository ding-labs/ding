# Ding watch preview

The default `ding` binary now runs persistent watches. It supports HTTP, command
and authenticated push sources; typed and numeric conditions; transition/recovery,
change, provider event and missing-data policies; SQLite state/evidence; and durable
console/webhook/Slack/Discord delivery.

This is an architectural cutover. `run`, legacy `serve`, `test-rule` and `install`
provide migration guidance instead of running the old engine. Use the hardened
v0.14.0 release for legacy workloads. `ding migrate` writes supported manifests and
an explicit per-rule report; it does not start them or import legacy snapshots.

No watch stable release or performance capacity is declared by these notes.
Review the qualification report and release checklist before tagging artifacts.
