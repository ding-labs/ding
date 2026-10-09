# Persistent watches. Durable alerts.

Ding watches an HTTP endpoint, runs an explicit local check, or accepts JSON from
your software. It keeps condition state, records why an event happened, and retries
notifications through a durable outbox. The Go daemon runs on your infrastructure.

[Create your first watch](guides/first-watch.md){ .md-button .md-button--primary }
[Choose an installation](install.md){ .md-button }

## Start with a question

| You want to… | Start here |
| --- | --- |
| See a firing and recovery without an external service | [First watch](guides/first-watch.md) |
| Watch an API for repeated failures | [HTTP monitoring](guides/http.md) |
| Notice a missing heartbeat | [Missing data](guides/missing-data.md) |
| Run a trusted local check | [Command sources](guides/command.md) |
| Understand why an alert happened | [Investigate an event](guides/investigate.md) |
| Operate a persistent daemon | [Operating Ding](operate/index.md) |
| Use the forthcoming interface | [Console availability](console/index.md) |
| Keep or migrate an old installation | [Legacy v0.14.0](legacy.md) |

## One watch, four connected stages

**Observe → evaluate → record → deliver.** A source supplies an observation.
The daemon evaluates the condition, commits state and evidence to SQLite, and
records delivery work in the same transaction. Notification workers retry that
committed work across restarts.

A running watch, an open incident, a source error, and a failed delivery are
separate facts. [Learn the state model](concepts/index.md) before interpreting a
status or changing a watch.

## For people and agents

Use versioned YAML and the CLI/API today. An agent can help author and inspect the
same declaration; the daemon does not call a model on each check. Consumer data
such as flights or sports needs an actual provider feed and credentials.

Ding is self-hosted and Apache-2.0 licensed. [Ding Console](console/index.md) is
being implemented as an interface bundled with the daemon.
