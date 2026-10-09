# Examples

Start with `ding.yaml.example` at the repository root: authenticated push latency
readings, one firing transition, one recovery, console delivery. The repository
README walks through the daemon, apply and curl commands.

The `examples/watches/` directory also contains:

- `api-health.yaml`: poll a supplied health URL every five seconds, trigger after
  three actual 5xx responses, recover after two nonmatching observations.
- `command.yaml`: explicit trusted local argv, bounded JSON output and selected
  fields. Replace its example path for your machine.
- `provider-events.yaml`: stable provider event IDs with a declared dedup horizon.

Replay the health fixture without I/O:

```sh
ding test examples/watches/api-health.yaml --events testdata/watches/api-health.jsonl --json
```

Use `skills/ding-watch/SKILL.md` to author watches with an agent. It does not add
an LLM call to the runtime or invent data-provider access. The
[authoring demonstration](development/inspection-contract.md#authoring-demonstration)
is exercised through the public CLI in tests.
