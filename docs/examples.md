# Watch examples

Use the [watch source preview](install.md). Each example is a versioned manifest
that you can validate and inspect before applying. Start with the
[first-watch tutorial](guides/first-watch.md) for a complete local firing and recovery.

| Example | Input and condition | Guide |
| --- | --- | --- |
| Request latency | Authenticated push; value above 300 ms | [First watch](guides/first-watch.md) |
| API health | HTTP polling; three actual 5xx responses | [HTTP monitoring](guides/http.md) |
| Local status | Trusted explicit command returning JSON | [Command source](guides/command.md) |
| Job heartbeat | Ten minutes without fresh input | [Missing data](guides/missing-data.md) |
| Deployed version | Typed value changes after a baseline | [Changes](guides/changes.md) |
| Provider events | Stable IDs and a declared deduplication horizon | [Provider events](guides/changes.md#deduplicate-provider-events) |

The docs build includes the canonical manifests as downloads. Substitute real
endpoint URLs, executable paths, and environment references before live use.
Examples are configurations, not bundled access to third-party data.

## Test without a live source

From a built source checkout:

```sh
./ding test examples/watches/api-health.yaml \\
  --events testdata/watches/api-health.jsonl --json
```

The fixture produces one firing and one recovery with no source requests or
notifications. Keep fixture input separate from your live watch stream.

## Author with an agent

The repository's
[authoring skill](https://github.com/ding-labs/ding/blob/main/skills/ding-watch/SKILL.md)
can help translate a supported request into the same manifest and normal CLI
workflow. It does not add a model call to the daemon or invent provider access.
Validate, explain, replay a fixture, review a dry-run, then apply within the
operator's requested scope.
