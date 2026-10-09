# Understand why an event happened

Use the watch preview and the same state directory as the running daemon.

```sh
./ding events --watch latency --state-dir ./ding-state --json
./ding events inspect EVENT_ID --state-dir ./ding-state --json
./ding events observations EVENT_ID --state-dir ./ding-state --json
```

Replace `EVENT_ID` with the selected event's ID. Inspect its definition revision,
triggering input, prior checkpoint state, and replay status. Explain a historical
event with its historical definition, even if the watch has since changed.

`verified` means the recorded checkpoint reproduces the event. It is not a
cryptographic authenticity claim. Lifecycle events and older records can lack
replay evidence. A prior matching count in a checkpoint is not proof that all
those raw observations are still retained.

## Verify offline

```sh
./ding events inspect EVENT_ID --state-dir ./ding-state --json > evidence.json
./ding replay evidence.json --json
```

Replay performs no source request or delivery. Evidence can contain selected
source fields; handle exported files according to the sensitivity of that data.

## Follow and page through history

```sh
./ding events --watch latency --follow --state-dir ./ding-state --json
```

Stopping follow stops the reader, not the watch. Save opaque cursors after
consuming their pages. A cursor belongs to its store and watch filter. A
`cursor_expired` error means history was removed; restart from available history
only after acknowledging that gap. Inspection endpoints also paginate entities,
observations, and deliveries. See the [inspection contract](../development/inspection-contract.md).
