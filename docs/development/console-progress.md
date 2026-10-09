# Console implementation progress

The [plan](console-plan.md) and [architecture decisions](console-architecture.md) define scope. The existing runtime soak is separate and is not restarted by console development.

| Phase | Status | Evidence |
| --- | --- | --- |
| U01 Design/contracts | Complete | Evidence-first selected; screen and failure-state contracts; repository/deployment decision; CLI inventory test |
| U02 Foundation/auth | Complete | Race-tested browser boundary (origin, Host, CSRF, one-use/expired handoffs, logout, session expiry, quotas, ingest isolation); static/deep-link tests; generated contract drift test; headless and embedded builds; Chromium real-daemon launch/deep-link/theme/logout test |
| U03 Operational reads | Complete | Bounded watch/event/delivery/destination reads, complete attention counts, filter-bound cursors and retention-gap tests; evidence from event-time definitions; real-daemon Chromium firing/evidence/delivery journey; race-tested store/control/runtime and generated contracts |
| U04 Workbench/apply | Pending | |
| U05 Lifecycle/delivery | Pending | |
| U06 Complete parity | Pending | |
| U07 Qualification | Pending | |

The parity inventory records the target screen for every runnable CLI command. It is not a claim that an unimplemented screen works. Completion requires the phase gates and end-to-end verification.
