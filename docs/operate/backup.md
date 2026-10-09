# Back up, restore, and upgrade

A backup is created on the **daemon host**, not necessarily the machine running
the client. Choose a new absolute output path with enough free space.

```sh
./ding backup --out /absolute/private/ding-backup.db --state-dir ./ding-state --json
```

Ding creates a consistent SQLite backup, verifies it, and publishes it without
overwriting an existing file. A failed backup publishes no partial result. The
operation serializes database work; run it during low load.

## Restore

1. Stop the daemon cleanly.
2. Create a new private state directory with appropriate permissions.
3. Copy the verified compatible backup into that directory as `ding.db`.
4. Start the appropriate binary with `--state-dir` pointing to the new directory.
5. Inspect Doctor, watches, events, and pending delivery before returning producers.

New directories receive new API credentials; update producers deliberately.
Credentials are separate from the database. Pending delivery can resume and send
notifications when the restored daemon starts. Avoid running the original and a
restored copy against the same producers or destinations unintentionally.

## Upgrade and rollback

Read release notes and compatibility instructions, record the old binary version,
and take a verified backup before replacing it. Never run an older binary against
a newer database schema. Roll back by stopping the daemon and restoring a
compatible backup into a new private directory. Keep the original state available
until the restored instance is verified.

A manifest export is useful for configuration review but does not contain state
or evidence. [Legacy conversion](../legacy.md) also does not import old snapshots.
