# First-watch qualification — November 5, 2026

This run qualifies saved-destination reuse and delivery on an actual Mac and the
deployed Ding Cloud runtime. It does not mark every adoption-plan acceptance
criterion complete. Times below are UTC on November 6 (November 5 in Pacific time).

## Source and automated checks

Local and hosted onboarding can select a saved destination, including later
pages, without redefining it or collecting its credentials again. Preview binds
the destination revision; tests and apply reject stale revisions. Test receipts
are persisted before delivery, and replaying the same operation never resends.
An unresolved transport outcome stays uncertain instead of claiming success.
The UI requires explicit receipt confirmation before activation.

Verified in the working checkout:

- Go race tests and vet for onboarding, store, watchrun, control, cloud, and access.
- All 35 Console unit tests, the production build, and all five Chromium
  onboarding tests. The saved-destination browser scenario loses a real test
  response, reconciles with the same key, and verifies the destination is unchanged.
- `make test-mcp`: embedded-app build/unit tests, tagged Go race tests, independent
  TypeScript SDK interoperability, and both embedded-app browser tests.

The checkout also contains pre-existing uncommitted changes. The development
binary includes that working tree; this is not qualification of a clean release
artifact. Implementation commits preserve the unrelated changes unstaged.
`npm ci` reported two high-severity audit findings in existing MCP-app dependencies
(`undici` and `zod`); dependency remediation remains a separate follow-up.

## Actual Mac

Environment: Apple Silicon, macOS 27.0 (26A428). A fresh Console-enabled Ding
binary and ad-hoc-signed native helper ran beside each other in an isolated
temporary directory. The legacy `ding` on PATH and the user's normal state were
not replaced. This does not qualify Developer ID signing or notarized installation.

- macOS reported `authorization: authorized` and `alerts: enabled`.
- A saved desktop destination accepted the labeled setup test at
  `00:47:26.089117Z`. The user explicitly confirmed seeing it as a native
  notification. Replaying the operation before and after daemon restart returned
  the same original receipt.
- The unmodified local starter recipe checked the synthetic loopback fixture
  every 30 seconds with a five-second timeout, three matches to fire, two
  nonmatches to recover, and `hold-incident` on unknown input.
- Three HTTP 503 observations produced one firing event at `00:50:01.909921Z`.
  Its desktop delivery was accepted by the OS on its first attempt. The open
  incident survived stopping and restarting the daemon with the same state.
- Timeout mode produced `source_error` at `00:50:36.914139Z`; the incident remained
  open. HTTP 200 then produced `source_recovered` at `00:51:06.917662Z` while
  awaiting the second nonmatching observation for incident recovery. That second
  observation recovered the incident at `00:51:36.922034Z`.
- All four runtime notifications were accepted by macOS on attempt 1. Firing and
  recovery evidence both returned `replayStatus: verified`. The temporary watch
  was paused before stopping the fixture and daemon; its isolated state was
  retained for inspection.

Only the setup test has explicit human visibility confirmation. Runtime delivery
receipts mean OS acceptance; they do not prove the user read each incident alert.

## Deployed Ding Cloud and Slack

The existing authenticated **Ding — Test** connector inspected capabilities and
created `qualification-saved-destination-20261105` in the current personal
workspace. The user explicitly authorized qualification alerts to
`#ding-notifications`.

The manifest referenced `slack-ding-notifications`. Preview and apply both
reported no destination changes. Its revision remained
`e5f7b9026f2e0bd3191dadd59b9a2e5e163ec5d8236b332bfaede3c5d5dc04d1` after the test.
No webhook secret was retrieved or replaced.

The synthetic watch polled Ding Cloud's public `/healthz` every five seconds.
`count(value) over 9m < 5` on `http.status` deliberately matched during sample
warmup, then stopped matching. The source stayed HTTP 200: this tested hosted
scheduling and incident transitions without causing a real service outage.
Every alert explicitly identified the sample-count qualification scenario.

| Event | Time (UTC) | Evidence |
| --- | --- | --- |
| Firing | 00:47:50.906413148 | Event `32e0a68d62afbe240292708592f8469766b3b06ca7a3becfe787be512bb90fd6`, deterministic replay verified; delivery 4 accepted on attempt 1; [Slack message](https://devrel-flywheel.slack.com/archives/C0C8G0KVBMH/p1793926071066189) read back from the channel |
| Recovered | 00:48:09.070045213 | Event `f5332e283becd5436a8e95acdc852e9fc742980319c1086c73ece2082200f51c`, deterministic replay verified; delivery 5 accepted on attempt 1; [Slack message](https://devrel-flywheel.slack.com/archives/C0C8G0KVBMH/p1793926089195349) read back from the channel |

The qualification watch was paused at generation 2 after recovery, retaining its
events and delivery history. Existing watches were not modified.

## Remaining qualification

- The new onboarding code was tested locally; it has not been deployed to Ding
  Cloud. This hosted run qualifies the deployed runtime and existing connector,
  not the new hosted form or saved-destination test endpoint in production.
- Fresh OAuth installation, Claude host behavior, laptop-off execution, click-to-
  evidence navigation from a native notification, and signed installer behavior
  remain separate checks. The laptop stayed awake during this run.
- Pilot activation/retention, paid operations, and assistant event wakeups retain
  the acceptance criteria in the [adoption plan](developer-adoption-plan.md).
- The two 24-hour soaks remain optional, non-blocking background validation.
  Their completion is not a release/deploy gate and requires no per-release waiver.
