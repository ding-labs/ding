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
