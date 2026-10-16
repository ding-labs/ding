# Choose a server database

SQLite is the default. The personal `ding daemon` uses SQLite. A shared
`ding server` or `ding-cloud` installation can explicitly select PostgreSQL in
its private server configuration. Both drivers ship in the same Go application;
a PostgreSQL deployment needs no Ding Cloud or Supabase account.

PostgreSQL currently supports **one active Ding server per database**, with direct
connections. Another server or offline operator is refused while that owner is
active. Horizontal workers and transaction poolers need separate qualification.

## SQLite

Omit `database`, or set `"database": {"backend": "sqlite"}`. Existing `dataDir`
behavior remains: one control database and separate workspace databases. SQLite
uses WAL with FULL synchronous durability. No PostgreSQL service is needed.

SQLite upgrades back up existing workspace files before migrating them. Stop the
server and use its encrypted backup operation before upgrading an installation.
The current SQLite workspace cap remains within the qualified 128 MiB offline
backup envelope.

## Fresh PostgreSQL installation

Use PostgreSQL 17 with durable storage. The local contract tests use PostgreSQL
17.11 with `fsync`, `full_page_writes` and `synchronous_commit` enabled. Budget
memory, connections, disk and backups for the database as well as the Go service.

Create separate application and migration roles. As the database administrator,
use interactive password prompts or your deployment's secret manager:

```sql
CREATE ROLE ding_migrator LOGIN;
CREATE ROLE ding_app LOGIN;
\password ding_migrator
\password ding_app
CREATE DATABASE ding OWNER ding_migrator;
```

Connect to `ding`, then restrict public creation:

```sql
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

The application role must not own the database or Ding schemas, inherit their
owner roles, or have superuser/BYPASSRLS privileges. Migrations grant the required
DML access. Runtime workspace tables use forced row level security and scoped
constraints. Use a dedicated database for this Ding instance.

Create an owner-only `server.json` with absolute paths:

```json
{
  "hosting": "self-hosted",
  "publicURL": "https://ding.example.com",
  "listen": "127.0.0.1:8787",
  "dataDir": "/var/lib/ding-server",
  "keyFile": "/etc/ding-server/master.key",
  "database": {
    "backend": "postgres",
    "dsnRef": {"env": "DING_DATABASE_URL"},
    "maxOpenConns": 8
  }
}
```

Supply `DING_DATABASE_URL` through the service's private environment. Use a direct
PostgreSQL URL; remote connections require verified TLS, normally
`sslmode=verify-full` with the appropriate trusted CA. Local loopback/socket
connections can use `sslmode=disable`. Keep passwords out of JSON, command
arguments and logs. Database errors are redacted by the Go adapter.

`maxOpenConns` bounds the shared query pool, defaults to 8 and accepts 1–64.
There is also one dedicated direct connection for execution ownership. Increasing
the pool does not parallelize writes within a workspace: evaluation and sequence
allocation remain ordered by that workspace's transaction lock.
The pool retains up to its configured cap between bursts, closes connections idle
for one minute, and retires reusable connections after five minutes.
`queryMode` defaults to `exec`. Set it to `cache_statement` to compare pgx's bounded
512-entry prepared-statement cache per connection on direct PostgreSQL. Workspace
scope remains transaction-local. This option does not qualify a transaction pooler;
execution ownership still requires a dedicated direct connection.
Size the pool against the combined Go/database CPU budget. In the
[local qualification](../development/push-scale-results.md), a pool of 2 passed
the tested one-CPU hot-workspace profile; larger pools were not consistently
faster. The report's workload, raised lab policy limits and storage caps apply.

Prepare the private configuration/password files and writable directories, then:

```sh
ding server --config /etc/ding-server/server.json run --check-config
ding server --config /etc/ding-server/server.json migrate \
  --database-admin-env DING_MIGRATION_URL
ding server --config /etc/ding-server/server.json init \
  --username operator --password-file /etc/ding-server/initial-password
ding server --config /etc/ding-server/server.json run
```

`DING_MIGRATION_URL` identifies the same database using the migration role. Supply
it only to offline migration commands; normal startup uses the application role.
Stop the service before later migrations. Startup checks the exact schema,
instance identity, role and durability, and refuses incompatible state.

`init` creates the private 32-byte credential master key if it is absent. Preserve
that key separately from the database and its backups. Put a trusted HTTPS proxy
in front of the loopback listener; apply the existing shared server authentication
and source/egress policy independently of database selection.

The data directory still holds private instance binding and operator artifacts.
Do not reuse a personal daemon directory. A failed PostgreSQL connection stops
startup; it never opens an empty SQLite replacement.

## Storage and monitoring

SQLite reports file/page/WAL usage. PostgreSQL's per-workspace quota measures
canonical UTF-8 JSONB row representations, excluding workspace identifiers; binary
columns use the configured hex representation. This logical quota excludes indexes,
TOAST compression, WAL, vacuum overhead and shared control records. It is not a
physical disk limit and is not numerically interchangeable with SQLite file bytes.

PostgreSQL defaults to the existing 128 MiB workspace logical cap; an operator can
set `capacity.maxStoreBytes` up to 1 TiB. Reserve and monitor physical database,
index and WAL space separately. The private operator `/metrics` endpoint exposes
bounded operation timings, SQL pool pressure and worker totals without tenant or
payload labels. Doctor reports backend/schema and workspace health.

## PostgreSQL backup and restore

Stop Ding first. Install matching PostgreSQL client tools on the operator host:
`pg_dump` must support the server version. Optional `DING_PG_DUMP` and
`DING_PG_RESTORE` references select absolute executable paths. Minimal server
container images do not include these operator tools.

Backup/restore additionally require a private administrator DSN with superuser or
BYPASSRLS permission, because a complete archive must include every workspace.
Keep this privilege out of the normal service environment. Generate and retain an
age backup identity separately:

```sh
ding server backup-keygen --output /secure/ding-backup-identity
```

The command prints its public recipient. Use that public value for backup:

```sh
ding server --config /etc/ding-server/server.json backup \
  --database-admin-env DING_BACKUP_ADMIN_URL \
  --output /backups/ding.age --recipient AGE_PUBLIC_RECIPIENT
```

The native custom-format dump streams directly into age encryption. Restore to a
fresh data directory and empty destination database using a destination config
and the **original external credential master key**:

```sh
ding server --config /etc/ding-server/restore.json restore \
  --database-admin-env DING_RESTORE_ADMIN_URL \
  --input /backups/ding.age --identity-file /secure/ding-backup-identity
```

Restore preserves durable receipt identity and sequence ranges, then quarantines
the destination: watches pause, sessions/old credentials are revoked and stale
pending work is canceled. A database-level quarantine survives loss of the local
marker. A failed or unauthenticated archive cannot be released. Source workspace
byte caps are reset so quarantine can complete even from a full source; starting
the destination reapplies its configured caps.

Native restore creates objects under the restore operator role. Future offline
migrations must use that owner/admin role, or the administrator must deliberately
transfer ownership to the migration role. Keep the application role separate.

After reviewing the restored instance and verifying all other runners are stopped:

```sh
ding server --config /etc/ding-server/restore.json release-restore \
  --confirm-other-runners-stopped
```

Release keeps watches paused. Review and resume them through the existing account
workflow with new credentials. Managed snapshots/PITR and physical power-loss
recovery require qualification on the intended deployment.

## Existing SQLite installations

Changing `database.backend` does not transfer data. The instance binding refuses
an implicit backend or database switch. Fresh PostgreSQL self-hosting is available;
a populated SQLite-to-PostgreSQL cutover still requires the explicit export/import,
receipt reconciliation and rollback rehearsal in the
[database migration plan](../development/push-scale-postgres-plan.md#g7-select-the-cloud-backend-and-rehearse-migration).
The active Ding Cloud soak remains on its existing SQLite deployment.
