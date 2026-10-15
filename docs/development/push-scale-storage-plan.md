# Configurable SQLite and PostgreSQL storage

This extends the [performance plan](push-scale-plan.md) with the owner's explicit
requirement to support both databases for open source self-hosting. SQLite stays
the default. PostgreSQL is an optional backend in the same Go application and
published server image; it requires no Ding Cloud account or Supabase account.
The current implementation remains SQLite-only until the adapter work lands.

## Shared domain logic and backend owned SQL

The HTTP/CLI services call a shared Go store/transaction contract for domain
operations such as loading entity state, recording observations, saving receipts
and claiming deliveries. Authorization and evaluation remain shared Go code.

Each backend owns its SQL, row mapping, migrations and transaction implementation.
Use distinct `sqlite_*` and `postgres_*` implementation files behind the existing
`internal/store` facade initially, with the corresponding separation in
`internal/serverstate`. Put SQL/migration assets under backend-specific paths.
Extract subpackages only when the dependency boundary avoids circular imports.

| Difference | Backend responsibility |
| --- | --- |
| Parameters | SQLite `?` parameters; PostgreSQL `$1`, `$2`, and so on |
| Inserts and conflicts | Explicit dialect-specific insert/upsert/returning statements and identical domain results |
| JSON and binary fields | Appropriate backend types/functions, retaining canonical bytes where identity depends on them |
| IDs and sequence ranges | SQLite's current behavior; transactional PostgreSQL workspace range allocation |
| Locking and snapshots | SQLite writer gate and read snapshots; PostgreSQL transactions and row/lease fences |
| Storage and maintenance | SQLite page/WAL/checkpoint operations; PostgreSQL logical quota, vacuum/WAL and physical reserve reporting |
| Audit and retention | Backend-specific mutation accounting, staging/reference operations and portable audit behavior |

For example, a logical `Entity(workspace, watch, key)` read uses separate SQL:

```sql
-- SQLite: the selected file already scopes the workspace.
SELECT state FROM entities WHERE watch_id = ? AND entity_key = ?;

-- PostgreSQL: the shared schema must also scope the workspace.
SELECT state FROM ding_runtime.entities
WHERE workspace_id = $1 AND watch_id = $2 AND entity_key = $3;
```

Keep this duplication confined to data access. Avoid spreading backend checks
through handlers/evaluation or treating placeholder substitution as a complete
port. Use explicit SQL implementations for operations with different semantics.
Backend-specific optimizations remain behind the common behavioral contract.

## Instance configuration

Add an optional `database` object to the existing owner-only server JSON config.
The following fragments are proposed configuration, not currently accepted fields.
Omitting the object preserves existing SQLite configuration and `dataDir` behavior.

```json
{
  "database": {
    "backend": "sqlite"
  }
}
```

```json
{
  "database": {
    "backend": "postgres",
    "dsnRef": { "env": "DING_DATABASE_URL" },
    "maxOpenConns": 8
  }
}
```

Resolve the referenced DSN privately. Validate TLS and connection settings for the
deployment, keep secrets out of logs and process arguments, and require explicit
backend selection. An environment variable alone must not silently switch an
existing SQLite installation to PostgreSQL. Reject unknown/incompatible options.

Select one backend for the whole server instance initially, including control
metadata and all workspace stores. PostgreSQL uses one bounded instance pool
shared by scoped store facades; avoid a new eight-connection pool for every
workspace. Root shutdown owns the shared pool; closing one tenant cannot close
another tenant's connections. Migration/backup roles have separate bounded access.

Hosting mode and database backend are independent choices. Both `self-hosted`
and `cloud` hosting can select either supported backend. The personal local
daemon retains its SQLite default; a person wanting a PostgreSQL-backed shared
installation runs the self-hosted server and connects through the normal clients.
Clients receive no database credentials and keep the same HTTP/API experience.

## Startup and operator commands

Create the backend through one composition-root factory and pass its scoped
stores to the existing service. Apply that factory consistently in `cloud.Run`,
tenant opening and `ding server run/init/recover`. Route backup, restore,
workspace deletion and maintenance through the same selected backend; an operator
command must not accidentally open SQLite after a server selected PostgreSQL.

Preserve `dataDir` for SQLite and any required private local operator artifacts;
keep the encryption key external to backups under the existing key policy.
Backend readiness validates connectivity, schema version, instance identity and
durability. PostgreSQL startup does not require a workspace SQLite directory or
SQLite file-lock probe. A configuration or database failure stops startup.

Use explicit migration locking so two starting processes cannot race schema
changes. Initially qualify PostgreSQL with one server process and enforce that
ownership; enable multiple owners only after distributed leases/fences pass.
Require maintenance operations to quiesce every active owner.

The shared PostgreSQL pool must not be reentered for a second connection while a
transaction holds the first one. Execute final authorization/accounting checks
through the same database transaction, including cross-schema control queries.
Test tiny pool limits to expose accidental nested checkouts and deadlocks.

## Separate migrations with one semantic contract

Keep versioned SQLite and PostgreSQL migration streams. PostgreSQL's initial
schema includes tenant-scoped constraints and receipt-range allocation rather
than a mechanical translation of SQLite's schema. Database-specific versions
and a shared semantic contract version are visible to operator diagnostics.

Run the same domain tests against both stores in CI. Changes to a shared operation
must pass both implementations. Add dialect-specific tests for migrations,
locking, snapshots, quota/durability and backup behavior. Compile both drivers
into the normal open source binary/image; SQLite startup opens no PostgreSQL
connection and requires no running PostgreSQL service.

Do not grow backend checks in business logic to accommodate exceptions. If an
operation needs different execution semantics, extend the store contract with a
named capability/operation and test each implementation against its guarantees.
Keep backend-specific tuning in operator configuration, with bounded defaults.

## Switching existing installations

Configurable startup selects storage; it does not copy or migrate existing data.
Support a fresh PostgreSQL installation through normal self-hosted initialization.
For an existing installation, require an explicit backend-aware export/import or
migration workflow with source and destination configurations.

Validate expected instance identity and migration completion before running the
destination. Quiesce the source, preserve its encrypted backup, import consistently,
reconcile receipts/usage/evidence, and enable only one destination. Honor restore
quarantine and session/grant invalidation. Test reversal before new destination
writes; later rollback requires transfer/reconciliation of those writes.
The [PostgreSQL migration gates](push-scale-postgres-plan.md#g7-select-the-cloud-backend-and-rehearse-migration)
describe the recovery requirements.

## Self-hosting qualification

Provide example server configurations and Compose variants for SQLite and
PostgreSQL. Test fresh initialization, startup/restart, login, ingest, inspection,
credential lifecycle, backup/restore and deletion through the actual self-hosted
server, then repeat the cloud-hosting suite against the same backend. Preserve
each hosting mode's existing source/egress/command policy independently of storage.

Also test omitted backend, invalid backend, missing DSN reference, malformed DSN,
unreachable database, TLS mismatch, incompatible schema, migration failure, a
backend switch without migration and redaction of diagnostics. A failed PostgreSQL
connection must never cause silent fallback or an empty replacement SQLite store.

**Exit:** an open source operator can use either supported backend by configuring
the same server distribution, with tested initialization and recovery instructions.
General PostgreSQL works without adopting Supabase Auth, REST or Edge Functions;
Supabase compatibility is an additional deployment test using that same adapter.
