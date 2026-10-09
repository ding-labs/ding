# Ding Console availability

Ding Console is being implemented. The watch source snapshot documented here does
not include the interface or the `ding ui` command. Use the
[first-watch CLI walkthrough](../guides/first-watch.md) for working instructions.

## The selected direction

The Console will lead with evidence: what was observed, how the condition was
evaluated, what event was recorded, and what happened to delivery. It is planned
to ship inside the Go executable and connect to the serving daemon.

| Planned area | Purpose | Working CLI path today |
| --- | --- | --- |
| Watches | Find attention, inspect state, pause and resume | `watch list`, `watch inspect`, `watch pause`, `watch resume` |
| Events | Read history and inspect evidence | `events`, `events inspect`, `events observations` |
| Deliveries | Inspect attempts and retry eligible failures | `delivery inspect`, `delivery retry` |
| Workbench | Validate, test, review, apply, replay, migrate | `validate`, `explain`, `test`, `apply --dry-run`, `apply`, `replay`, `migrate` |
| System | Diagnose, export, back up, inspect version | `doctor`, `export`, `backup`, `version` |

## Access and state

The planned local launch flow establishes a short-lived browser session through
the CLI. The public website will not ask for daemon tokens or connect to localhost.
Remote access requires explicit HTTPS configuration. No hosted account is implied.

Operational help will distinguish condition, source, delivery, and browser
connection state. Lost browser connectivity does not mean the daemon stopped.
Unknown input can coexist with an open incident; pausing a watch can leave delivery
in progress. These distinctions already apply to the CLI/API.

Task instructions and real screenshots will be published only after their matching
console operations and executable pass verification. The website's Console
illustration is labeled as a design preview.
