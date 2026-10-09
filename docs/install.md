# Installation

The watch runtime is a source preview. Released v0.14.0 installers still contain
the legacy CLI. Do not use a legacy binary with a watch manifest.

From this checkout with Go 1.26:

```sh
go build -trimpath -ldflags='-s -w' -o ding ./cmd/ding
./ding version
./ding daemon --state-dir ./ding-state
```

The daemon owns one local state directory. Back it up through `ding backup`; a
second daemon cannot share it. To stop, send SIGINT/SIGTERM. Pending delivery
resumes on next start. No operating-system service installer is included yet;
run the foreground command under your preferred service manager.

Build a local container with `docker build -t ding-watch .`. The default command
is `daemon --state-dir /var/lib/ding`, binding loopback inside the container.
Mount a persistent private volume at `/var/lib/ding`. For host access, explicitly
pass `daemon --state-dir /var/lib/ding --listen 0.0.0.0:7676 --allow-remote`, map only
the intended host interface, and configure TLS/authentication at your proxy.
The image contains Ding and CA roots; command sources need executables supplied
by your derived image or mounted into it.

The planned artifact matrix is Linux, macOS and Windows on amd64/arm64. Stable
support claims require the native qualification results, not cross-compilation
alone. Package sizes and measurements will be recorded at qualification.

For existing installations, [legacy v0.14.0 and migration](legacy.md) remain available.
