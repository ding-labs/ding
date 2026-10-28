# Persistent watches. Durable alerts.

Ding watches an HTTP endpoint, runs an explicit local check, or accepts JSON from
your software. It keeps condition state, records why an event happened, and retries
notifications through a durable outbox. Run public HTTP watches in Ding Cloud,
or install the daemon on your own infrastructure.

[Get started with Ding Cloud](https://cloud.ding.ing/auth/login){ .md-button .md-button--primary }
[Install locally](install.md){ .md-button }

Cloud keeps checking while your computer is off. Local installation supports
private endpoints and command checks without an account.
[Create your first local watch](operate/local-setup.md).

## Start with a question

| You want to… | Start here |
| --- | --- |
| See a firing and recovery without an external service | [First watch](guides/first-watch.md) |
| Watch an API for repeated failures | [HTTP monitoring](guides/http.md) |
| Notice a missing heartbeat | [Missing data](guides/missing-data.md) |
| Run a trusted local check | [Command sources](guides/command.md) |
| Alert on warnings or errors in application output | [Live log monitoring](guides/logs.md) |
| Understand why an alert happened | [Investigate an event](guides/investigate.md) |
| Operate a persistent daemon | [Operating Ding](operate/index.md) |
| Open the local graphical interface | [Ding Console](console/index.md) |
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
included in the v0.15.0 Homebrew package.
