# Install the watch preview

{{ availability }}

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

## Existing releases

The published v0.14.0 binary, Homebrew package, and default installer currently
provide the legacy runtime. They do not run watch manifests. Follow the
[legacy installation and migration guide](legacy.md) for that version.

The Console is not part of this documented source snapshot. Its installation
instructions will be enabled when a matching release is verified.
