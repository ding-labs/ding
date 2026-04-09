# Install

## Homebrew (macOS / Linux)

```bash
brew install zuchka/tap/ding
```

## Binary script

Downloads and installs the latest release to `/usr/local/bin`:

```bash
curl -sf https://start.ding.ing | sh
```

## Docker

```bash
docker run -v ./ding.yaml:/etc/ding/ding.yaml \
  ghcr.io/zuchka/ding
```

Runs on `linux/amd64` and `linux/arm64`.

## Manual binary download

Download a release from [GitHub Releases](https://github.com/zuchka/ding/releases), extract, and place the binary on your `$PATH`.

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
