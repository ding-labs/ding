# Tell-Tail

Understand a live log stream in your terminal or browser. Tell-Tail follows one
file or reads stdin, measures throughput and severity, and keeps a small sample
of recent errors in memory.

These guides cover **Tell-Tail 0.4.0**, the published release.
MCP diagnostics, the built-in demos, expanded format recognition, parsing coverage,
and severity histories are available in the installed binary.

## Install

With Homebrew:

```sh
brew install ding-labs/tap/tell-tail
```

Or with Cargo (Rust 1.88 or later):

```sh
cargo install tell-tail --version 0.4.0 --locked
```

[Download native binaries](https://github.com/ding-labs/tell-tail/releases/tag/v0.4.0)
for Apple Silicon on macOS 14+ or Linux x86_64/ARM64. The browser dashboard is
embedded in the binary; it needs no Node, Python, account, or hosted service.

## Inspect desktop MCP logs

On macOS, discover existing Codex Desktop and Claude Desktop MCP logs:

```sh
tell-tail mcp --open
```

The local dashboard shows reported failures, scoped recovery, a searchable
timeline, and source freshness. No client configuration change or restart is
needed. It reads logs; it does not call MCP servers or capture complete traffic.
For synthetic examples on Mac or Linux, run `tell-tail mcp --demo --open`.
See [MCP diagnostics](mcp.md) for explicit Linux log directories and source limits.

Existing Homebrew users can upgrade with
`brew update && brew upgrade ding-labs/tap/tell-tail`. Check `tell-tail --version`
for 0.4.0 or later before using MCP mode.

## Your first log stream

Follow new lines in a log file:

```sh
tell-tail app.log
```

Open the browser dashboard and include existing lines:

```sh
tell-tail app.log --from start --output web --open
```

No log file handy? Run the built-in synthetic demo:

```sh
tell-tail --demo --window 5 --output web --open
```

The browser dashboard stays available until Ctrl+C. In the terminal, press `q` to
quit, space to freeze the display, and `?` for controls. Following normally starts
at the end of a file; an empty display is expected until new lines arrive.

## Go further

- [MCP diagnostics](mcp.md): supported desktop logs, scoped issues, demo, and privacy limits.
- [Log format recognition](log-formats.md) and [live-stream interpretation](monitoring.md): profiles, coverage, and histories.
- [Inputs, outputs, and webhook alerts](usage.md): commands, severity rules, Prometheus, Slack, and Discord.
- [Browser dashboard](dashboard.md): rates, charts, recent errors, and display controls.
- [Apple unified logs](apple-unified.md): watch native macOS activity.
- [Operations and troubleshooting](operations.md): rotation, buffering, limits, and service setup.
- [HTTP and SSE API](web-server.md) and [frame reference](web-frames.md): integrate local metrics.

Statistics and recent error previews live in memory and reset on restart.
Log severity ratios describe received records; they do not establish application
health. Webhook delivery is best effort. [Ding](https://ding.ing/) provides
persistent watches, evidence, and durable notification retries.
