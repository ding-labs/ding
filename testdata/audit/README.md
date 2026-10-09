# Audit regression baseline

`reproduction_test.go.txt` preserves the October 8, 2026 audit probes without
adding knowingly failing tests to ordinary test discovery. Each implementation
step promotes the relevant probe into a focused regression test, replacing
timing-dependent stress checks with deterministic assertions where possible.

The probes cover cooldown clock/concurrency, label identities, snapshot window
compatibility, HTTP 429 outcomes, benchmark warmup, and idle label retention.
They exercise only local fixtures and never send a real notification.

The full baseline suite passed with Go 1.26.1 on darwin/arm64. `go vet ./...`
also passed. Advertised release targets are Linux, macOS, and Windows on amd64
and arm64. Cross-compilation verifies build support, not runtime support.

Implementation progress and validation results are recorded in
`docs/development/progress.md`.
