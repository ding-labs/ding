# Measure useful adoption without requiring an account

The primary outcome is a developer retaining a useful watch. A local or personal
server installation is as successful as a cloud installation. The software has no
required product analytics; this pilot uses explicit participants, observation and
voluntarily shared coarse outcomes. Download counts and GitHub signups are not
active-use counts.

## Local pilot first

Recruit a small consenting developer cohort across macOS, Linux and Windows.
Provide a qualified artifact, a blank machine/profile and a real endpoint each
participant owns. Observe installation, explicit startup consent, first watch,
notification permission/test confirmation and first real observation. Record elapsed
time and the precise step where help was needed. Do not record URLs, tokens,
commands, payloads or model transcripts. A fixture/demo run does not count as
activation. The initial hypothesis is a useful watch within five minutes.

Ask participants who consented to follow-up whether a useful watch still runs on
day 7 and day 30, and whether it is local, on their own server or hosted. Record
missing responses as unknown; report the sample/denominator and selection bias.
Do not infer inactivity from the absence of telemetry. Participants may remain
account-free throughout the pilot or withdraw without losing local functionality.

## Availability and cloud pilot

Watch details offer three explicit choices: this computer, the user's own server,
and Ding Cloud. Clicking cloud first checks eligibility locally and shows exactly
what would leave the machine. It does not create an account, upload a database or
quietly rewrite cadence. Only the user's subsequent choice invokes sign-in/move.

Record availability interest, eligible/ineligible result, sign-in started/completed/
canceled, destination test received, transfer prepared/finished/reconciled, first
hosted observation and voluntary return to local/self-hosted execution. Time from
cloud choice to actual hosted observation has an initial two-minute hypothesis
when credentials already exist. Interview people who keep Ding local or abandon
sign-in; their choice is useful adoption evidence, not automatically a failure.

Use a participant-chosen random code in a separately controlled pilot sheet.
Keep local retention observations separate from GitHub identity. The cloud operator
may aggregate operational reliability/cost without turning those records into a
cross-device product profile. Do not add silent tracking to improve funnel counts.

## Minimal outcome record

```json
{
  "cohort": "local-preview-1",
  "participantCode": "voluntarily-chosen-code",
  "platform": "macos-arm64",
  "artifactVersion": "0.15.0-preview.1",
  "installationCompleted": true,
  "serviceReady": true,
  "firstUsefulObservation": true,
  "notificationSeen": true,
  "activationSeconds": 210,
  "availabilityChoice": "own-server",
  "eligibility": "not-requested",
  "day7": "unknown",
  "day30": "unknown"
}
```

This is a template, not a measured result or automatic event payload. Keep optional
freeform feedback free of watch content; delete it on withdrawal under the pilot's
stated policy. Share aggregated results only with an adequate denominator.

Before cloud deployment, record the marketplace investigation outcome as confirmed
or unresolved, demand evidence, current provider quote and approved budget. Before
expanding a cloud cohort, review retained useful watches, reliable deliveries,
uncertain moves, auth abandonment, support effort and cost per retained watch.
Fix broken monitoring before optimizing conversion. Never make account creation a
prerequisite for installation, local MCP, updates, exports or personal-server use.
