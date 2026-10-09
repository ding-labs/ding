# Ding Console

The console is served by the same daemon as the versioned API. Its browser session can inspect and manage that instance; the public `ding.ing` website is a separate deployment.

## Build and open

From a checkout, use Go 1.26+ and Node 24+:

```sh
make console
./ding daemon
```

In a second terminal using the same state directory:

```sh
./ding ui
```

`ding ui --no-open` prints a launch link instead of opening a browser. The link expires after 60 seconds and can be used once. Sessions expire after eight hours or when the daemon restarts. Log out using the console's top bar. The long-lived admin and ingest tokens remain on the daemon host.

`make headless` and ordinary `go build ./cmd/ding` require no Node installation. Console-enabled builds use `go build -tags console` after `npm run build` in `web/console`. The build fails if the embedded production assets are absent.

## Development

Run the daemon with `--ui-origin http://127.0.0.1:5173` and start `npm run dev` in `web/console`. The development proxy forwards `/v1` to `127.0.0.1:7676`; `ding ui` opens the configured development origin. Both hosts must match the configured origin exactly. A console-enabled daemon is required for session launch.

The console uses generated Go API contracts. After changing public Go response/request types, run `go run ./cmd/console-contract` from the repository root. Contract drift is a failing Go test.

## Remote deployment

An API deployment using `--allow-remote` can continue without browser access. To enable a console behind a TLS reverse proxy, configure `--ui-origin https://your-console.example` and preserve that Host header when forwarding requests. The origin must contain no path, query or fragment. The daemon does not trust forwarded headers to decide whether a browser request is secure. The proxy must enforce HTTPS and protect access to the underlying HTTP listener.

Launch links returned by `ding ui` use the configured origin. SameSite cookies, exact Host/Origin checks and per-session CSRF headers protect browser mutations. The separate ingest bearer credential is still required for producers. Never forward long-lived credentials in website URLs or frontend configuration.

For design and implementation status, see the [console plan](development/console-plan.md) and [phase progress](development/console-progress.md).
