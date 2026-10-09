# Operate a Ding daemon

Run one daemon per private persistent state directory. Stop with SIGINT/SIGTERM;
pending committed work resumes on the next start. The source preview does not
include an operating-system service installer. Use your service manager to run the
foreground command under a dedicated account, with a stable working directory,
persistent volume, deliberate environment, and a restart policy.

```sh
./ding daemon --state-dir /absolute/private/ding-state
```

The directory contains the database, credentials, and local connection metadata.
Do not share it between daemons. [Back up through Ding](backup.md), not by copying
a database that is being written.

## Remote access

The API defaults to loopback and plaintext HTTP. To bind remotely, explicitly
select `--listen` and `--allow-remote`, then put an authenticated TLS reverse proxy
in front of the intended origin. Limit network access to that proxy and preserve
bearer authentication. Do not expose unencrypted credentials on a public listener.
The forthcoming Console has additional session/origin requirements; use its
version-specific documentation once released.

Command sources execute as the daemon user. Restrict its filesystem and network
permissions to the workload it needs. Environment references keep secrets out of
manifests, but literal arbitrary source fields and command arguments are not
reliably redacted: avoid putting secrets there.

## Limits and retention

Defaults are 1,000 active watches, 10,000 pending/leased deliveries, and 1 GiB of
live SQLite pages. Configure `--max-watches`, `--max-pending`, `--max-store-bytes`,
and `--history` deliberately. WAL files and backups need additional disk space.

Ordinary history expires after seven days. Active incident evidence, current
windows, and pending deliveries can remain pinned beyond that horizon. Receipt
keys last 24 hours. Store pressure applies backpressure; it does not silently
truncate an exact calculation. [Lifecycle contract](../development/lifecycle-contract.md).

## Day to day

Use [Doctor and troubleshooting](troubleshooting.md) for diagnostics, inspect failed
[deliveries](../guides/notifications.md), and take verified [backups](backup.md)
before upgrading. Qualify changed runtime behavior on the new executable; earlier
measurements do not automatically apply to a different binary.
