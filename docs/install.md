# Install Ding

{{ availability }}

## Homebrew (recommended)

On macOS 13+ or Linux, with an ARM64 or x86-64 machine:

```sh
brew install ding-labs/tap/ding
ding setup
```

The package includes the Console, MCP adapter and Mac notification helper. You
need no Go, Node, Python, model credentials or Ding account. Setup enables
background startup after your confirmation and opens the first-watch flow.
Follow [local setup](operate/local-setup.md) to create your watch and test an alert.

Existing Homebrew users: stop Ding if it is running, then `brew update` and
`brew upgrade ding-labs/tap/ding`. Run `ding setup` after the upgrade.

## Build from source

You need Git and Go 1.26. The quickstart uses a POSIX shell, curl, and jq.
These commands run on the machine that will host the daemon.

```sh
git clone https://github.com/ding-labs/ding.git
cd ding
git checkout {{ runtime_ref }}
go build -trimpath -ldflags='-s -w' -o ding ./cmd/ding
./ding version
```

This pins the runtime used by these instructions. To try newer development code,
use its documentation and record the commit you built. A source build may report
`dev`; the checkout commit identifies its behavior.

[Create your first watch](guides/first-watch.md) to start a daemon and test a firing
and recovery. Windows users can build `ding.exe` with Go; adapt the shell examples
to their terminal. See the [qualification report](development/qualification.md)
for actual platform results.

## Keep state private and persistent

The daemon owns one state directory. Pass the same `--state-dir` to all CLI
commands. It contains SQLite data, connection information, and separate admin and
ingest credentials. Never commit or publicly serve it. A second daemon cannot
share it. See [operations](operate/index.md) for limits and service management.

## Containers

From this checkout:

```sh
docker build -t ding-watch .
docker run --rm -v ding-state:/var/lib/ding \
  -p 127.0.0.1:7676:7676 ding-watch \
  daemon --state-dir /var/lib/ding --listen 0.0.0.0:7676 --allow-remote
```

The volume persists state. The host port is bound to loopback. The container's
listener must accept connections on its container interface. Command sources
require their executables in a derived image or a deliberate mount.

The container has its own connection and credential files; run CLI operations
inside it or explicitly configure an authenticated API client. Do not assume a
host CLI automatically reads a named container volume. Use [remote access](operate/index.md#remote-access)
when exposing the daemon beyond your machine.

## Earlier releases and other installers

v0.14.0 and earlier contain the old job-wrapper runtime. Follow the
[legacy installation and migration guide](legacy.md) only for those versions.
The old shell installer is not the v0.15.0 installation route; use Homebrew above.
Notarized Mac installers and signed Windows installers remain under qualification.
