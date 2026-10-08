# Install

## Homebrew (macOS / Linux)

```bash
brew install ding-labs/tap/ding
```

## Binary script

Downloads and installs the latest release to `/usr/local/bin`:

```bash
curl -sf https://start.ding.ing | sh
```

## Docker

```bash
docker run -v ./ding.yaml:/etc/ding/ding.yaml \
  ghcr.io/ding-labs/ding
```

Runs on `linux/amd64` and `linux/arm64`.

## Manual binary download

Download a release from [GitHub Releases](https://github.com/ding-labs/ding/releases), extract, and place the binary on your `$PATH`.

Available for:

| OS | Architecture |
|----|-------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| Windows | amd64, arm64 |

## Verify

```bash
ding version
ding --help
```

---

## Requirements

No runtime dependencies. DING is a statically linked binary. It runs anywhere.

### Container network access

The daemon binds loopback by default. A published container port requires a
config with `server.listen: 0.0.0.0`, plus distinct `admin_token` and `ingest_token`
values of at least 16 characters. Prefer `${DING_ADMIN_TOKEN}` and
`${DING_INGEST_TOKEN}` references and pass the variables with Docker `--env`.
Bind the host port to loopback (`-p 127.0.0.1:8080:8080`) for local use. Use a TLS
proxy for remote access. `/health` is public; other routes require their role's
bearer token. See [HTTP API](api.md).
