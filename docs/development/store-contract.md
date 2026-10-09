# Transactional store contract

P07 pins `modernc.org/sqlite v1.60.1` with its required `modernc.org/libc v1.77.1`.
It bundles SQLite 3.53.4 and supports the six advertised target combinations.
The package requires Go 1.26, matching this repository. The bundled SQLite version
includes the upstream WAL-reset fix. [Driver documentation](https://pkg.go.dev/modernc.org/sqlite),
[SQLite WAL documentation](https://sqlite.org/wal.html).

One process owns a state directory. Its persistent lock file uses an exclusive,
nonblocking OS lock: flock on Unix and LockFileEx on Windows. Closing or crashing
releases ownership; the lock file itself is never unlinked, avoiding replacement
inode races. Directory symlinks resolve before opening the lock/database. State
must live on a local filesystem, not an NFS/SMB share. Newly created directories
and files use owner-only Unix permissions; Windows deployments must keep the
state directory within a private user ACL.

SQLite runs in WAL mode with synchronous FULL, foreign keys enabled, a five-second
busy timeout, one database connection, and serialized store callbacks. The
schema records immutable watch/destination revisions, active lifecycle/schedule
pointers, observations, bounded-retention input receipts, entity state, timers,
events/evidence links, delivery intents, and delivery attempts. Indices cover due
work, event cursors, expiry, and per-watch/destination delivery order. Revisions
cannot be updated in place, enforced by database triggers.

`Store.Update` starts one transaction and commits only when the entire callback
succeeds. Observation insertion, source checkpoint, entity update, event/evidence,
input receipt, timer, and outbox intent can all participate. Callback failure or
commit failure returns an error; callers must not acknowledge input or update
an authoritative cache before success. `Store.View` always rolls back its read
transaction. Runtime coordination is introduced in P09; these store primitives do
not perform source or destination I/O.

Outbox claiming uses a random lease token and persists the attempt number before
returning the work. Expired leases become eligible again with the same event ID
and a new token. An old token cannot finalize reclaimed work. Pending/leased older
work blocks later work for the same watch/destination; other watches can proceed.
Intents refer to immutable destination revisions, so editing a destination cannot
rewrite already queued payloads or routing configuration. Secrets remain refs.
Actual credential resolution, retry-age policy, and worker dispatch are P09 work.

The current schema is 3. Its partial indexes cover only pending/leased queue
ordering and live leases, so delivery claims do not scan delivered history.
Reverse evidence and observation-expiry indexes bound retention work. Upgrades
from schema 1 or 2 use the same verified-backup and transactional migration path.

Schema migrations execute transactionally; a newer schema makes startup fail.
Upgrades of an existing schema create a verified backup first. Backups use
`VACUUM INTO` to produce a consistent copy including committed WAL contents,
check integrity, sync the output, and publish it without overwriting an existing
backup. On Unix the containing directory is also synced. [SQLite's backup semantics](https://sqlite.org/lang_vacuum.html).
Restore is offline: stop the daemon, preserve its whole state directory, create
a fresh directory, and put the verified backup at `ding.db`. Never copy only a
live main database file while ignoring its WAL.

Tests exercise actual subprocess exits before commit, after commit, and after
lease claim. They assert atomic recovery of all related tables and recovery of
expired work. Other tests cover cross-process ownership, destination revision
pinning, independent dispatch, explicit SQLite page exhaustion (`SQLITE_FULL`),
SQLite query-only write rejection, migration rollback, future-schema refusal,
backup/restore, and concurrent transactions. Physical ENOSPC and read-only mount
tests now run in isolated containers, and native Windows amd64/arm64 execution
passes. See the [qualification report](qualification.md) for exact tests and
remaining release gates.
