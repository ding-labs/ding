package state

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/ding-labs/ding/internal/database"
)

const PostgresSchemaVersion = 1

//go:embed postgres/001_control.sql
var postgresSchema string

func MigratePostgres(ctx context.Context, migration *sql.DB, appRole string) error {
	return database.Migrate(ctx, migration, "control", []string{postgresSchema}, appRole)
}

// OpenPostgres borrows the instance pool. The composition root owns shutdown
// and the instance execution fence; no per-workspace control pool is created.
func OpenPostgres(ctx context.Context, pool *sql.DB, owners ...*database.Ownership) (*DB, error) {
	var version int
	if err := pool.QueryRowContext(ctx, "SELECT version FROM ding_meta.versions WHERE component='control'").Scan(&version); err != nil || version != PostgresSchemaVersion {
		return nil, fmt.Errorf("PostgreSQL control schema is unavailable or incompatible; run server migration")
	}
	var unsafe bool
	err := pool.QueryRowContext(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR pg_has_role(current_user,n.nspowner,'MEMBER') OR pg_has_role(current_user,(SELECT datdba FROM pg_database WHERE datname=current_database()),'MEMBER') FROM pg_roles r JOIN pg_namespace n ON n.nspname='ding_control' WHERE r.rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		return nil, fmt.Errorf("PostgreSQL control requires a separate non-owner application role")
	}
	d := &DB{sql: pool, postgres: true}
	if len(owners) > 0 {
		d.ownership = owners[0]
	}
	return d, nil
}
