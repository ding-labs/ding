# Watch the service you just shipped

Use Ding to stop checking a health endpoint manually. Start with a service you
own and a concrete rule: notify after three consecutive server-error responses,
then notify when the incident recovers.

## Create and review the watch

Open the Console with `ding ui` after [local setup](../operate/local-setup.md),
or open your configured Ding Cloud Console. Choose **Create your first watch**.

1. Give the watch a recognizable ID and enter the service's health endpoint.
2. Choose notification delivery. Local setup offers desktop or Console-only
   delivery; cloud setup offers a webhook, Slack incoming webhook, or Discord
   webhook. Save destination credentials in the protected setup form. For local
   network destinations, use [Workbench](../console/index.md).
3. Preview the rule and review its endpoint, check interval, timeout, and policy.
   The description comes from the compiled definition. Local starter watches
   check every 30 seconds; hosted starter watches check every five minutes.
4. Send the labeled destination test and confirm you received it before starting
   desktop or hosted notifications. Console-only delivery records alerts inside
   Ding and the daemon output; it cannot interrupt you outside Ding.
5. Start the watch. The detail page initially waits for monitoring input. A saved
   configuration alone does not establish that the source is reachable.

The starter rule matches HTTP status 500 or above. It fires after three
consecutive matching observations and recovers after two nonmatching observations.
Responses such as 401 or 404 do not match this rule; a nonmatch does not establish
service health. If your endpoint must return exactly 200, use Workbench to set
`field: http.status`, `operator: ne`, and `value: 200`, then preview that change.

Three checks at a 30-second cadence span one minute from the first to the third;
at a five-minute cadence they span ten minutes. Waiting for the next check,
response time, scheduling, and delivery can add delay. Timeouts and unreachable
endpoints produce separate source errors. They do not silently recover an open
incident or count as successful checks.

## Know what is working

**Monitoring input received** means Ding has accepted a source observation.
Inspect the current condition separately: a 503 response is valid monitoring
input and can contribute to opening an incident. Inspect delivery separately too:
queued or failed notifications do not prove anyone received an alert.

Use **Inspect evidence** for recorded events and **Inspect deliveries** for
attempts and failures. The [investigation guide](investigate.md) explains the
difference between event-time evidence and the service's current state.

After input arrives, **Create another watch** starts a fresh setup with an empty
ID so you can monitor a different service without colliding with the first watch.
The availability section on the watch detail page explains the existing local,
self-hosted, and cloud choices. Local checking requires an awake, connected host.
Closing the Console or assistant does not stop the daemon. Hosted execution can
continue while your laptop is off, but cannot reach a private service on that
sleeping laptop. Review [execution locations](../operate/always-on.md).

## Practice with a controlled local service

This source-checkout fixture changes only its own synthetic endpoint. It does not
alter a real service. It binds to loopback and is therefore for a local Ding
instance, not a cloud watch. It is a development helper, not a new `ding` subcommand.

In a terminal at the repository root:

```sh
go run ./cmd/ding-health-fixture --listen 127.0.0.1:8787
```

Create a local watch named `fixture-health` for
`http://127.0.0.1:8787/health`. Use desktop delivery and confirm the test if you
want visible notifications, or Console-only delivery to inspect events. Wait for
**Monitoring input received**. In another terminal, change the fixture:

```sh
# Return HTTP 503. Wait for the three-check firing event before continuing.
curl --fail --request PUT --data failing http://127.0.0.1:8787/mode

# Stall HTTP requests until Ding's timeout. Wait for a source-error event.
# The existing incident must remain open.
curl --fail --request PUT --data timeout http://127.0.0.1:8787/mode

# Return HTTP 200. Wait for source recovery and two nonmatching observations.
curl --fail --request PUT --data healthy http://127.0.0.1:8787/mode
```

Expect one firing and one incident recovery, plus separate source-error and
source-recovery events. Keeping the endpoint at 503 does not repeatedly fire the
transition rule. In the Console inspect each event's evidence and its delivery.
Pause or delete only `fixture-health` when finished, then stop the fixture with
Ctrl+C. Stopping the fixture before the watch would deliberately create another
source error.

The regression scenario below uses both starter recipes, a real local HTTP
fixture and receiver, and persisted state. It verifies that consecutive counts,
the open incident, evidence, and queued delivery survive store reopening. It
advances acceptance time explicitly; it does not qualify real hosted scheduling,
OAuth, laptop-off operation, or human-visible desktop alerts.

```sh
go test -race ./internal/cloud -run TestFirstWatchRecipesFailureTimeoutRestartAndRecovery
```

For a shorter automatic demonstration through the real scheduler, use
`ding demo` or the Console's **Try a one-minute demo**. Both use temporary
synthetic state; neither counts as monitoring your own service.
