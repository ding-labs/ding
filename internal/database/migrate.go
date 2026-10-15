package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Migrate runs a component's explicit PostgreSQL migrations under one database
// lock and transaction. Use a separate migration role, never the application
// pool. The journal lives outside the application-writable schemas.
const metadataSchema = `CREATE SCHEMA IF NOT EXISTS ding_meta;
 CREATE TABLE IF NOT EXISTS ding_meta.versions(component text PRIMARY KEY,version integer NOT NULL CHECK(version>=0));
 CREATE TABLE IF NOT EXISTS ding_meta.owner(singleton integer PRIMARY KEY CHECK(singleton=1),pid integer NOT NULL,nonce text NOT NULL);
 INSERT INTO ding_meta.owner(singleton,pid,nonce) VALUES(1,0,'') ON CONFLICT DO NOTHING;
 CREATE TABLE IF NOT EXISTS ding_meta.lifecycle(singleton integer PRIMARY KEY CHECK(singleton=1),quarantined boolean NOT NULL DEFAULT false);
 INSERT INTO ding_meta.lifecycle(singleton,quarantined) VALUES(1,false) ON CONFLICT DO NOTHING;`

func Migrate(ctx context.Context, db *sql.DB, component string, steps []string, appRole string) error {
	if component != "runtime" && component != "control" {
		return fmt.Errorf("invalid database component")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SafeError("begin migration", err)
	}
	defer tx.Rollback()
	owner := ownershipFromContext(ctx)
	if err = owner.Fence(ctx, tx, true); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(68247531912805)"); err != nil {
		return SafeError("lock migration", err)
	}
	var unsafe bool
	err = tx.QueryRowContext(ctx, `SELECT rolsuper OR rolbypassrls OR rolname=current_user OR pg_has_role(rolname,current_user,'MEMBER') OR pg_has_role(rolname,(SELECT datdba FROM pg_database WHERE datname=current_database()),'MEMBER') FROM pg_roles WHERE rolname=$1`, appRole).Scan(&unsafe)
	if err != nil || unsafe {
		return fmt.Errorf("PostgreSQL application role must exist and cannot own or bypass migration security")
	}
	_, err = tx.ExecContext(ctx, metadataSchema)
	if err != nil {
		return SafeError("create migration journal", err)
	}
	var version int
	err = tx.QueryRowContext(ctx, "SELECT version FROM ding_meta.versions WHERE component=$1", component).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return SafeError("read schema version", err)
	}
	if version > len(steps) {
		return fmt.Errorf("PostgreSQL %s schema is newer than this binary", component)
	}
	for i := version; i < len(steps); i++ {
		if _, err = tx.ExecContext(ctx, steps[i]); err != nil {
			return fmt.Errorf("%s migration %d: %w", component, i+1, SafeError("migrate", err))
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO ding_meta.versions(component,version) VALUES($1,$2) ON CONFLICT(component) DO UPDATE SET version=excluded.version", component, len(steps)); err != nil {
		return SafeError("save schema version", err)
	}
	role := pgx.Identifier{appRole}.Sanitize()
	schema := "ding_" + component
	for _, query := range []string{
		"GRANT USAGE ON SCHEMA ding_meta," + schema + " TO " + role,
		"GRANT SELECT ON ding_meta.versions TO " + role,
		"GRANT SELECT,UPDATE ON ding_meta.owner,ding_meta.lifecycle TO " + role,
		"GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA " + schema + " TO " + role,
		"GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA " + schema + " TO " + role,
	} {
		if _, err = tx.ExecContext(ctx, query); err != nil {
			return SafeError("grant application access", err)
		}
	}
	if err = owner.Fence(ctx, tx, true); err != nil {
		return err
	}
	return SafeError("commit migration", tx.Commit())
}

// AllowMetadataRead lets the application prove that its DSN identifies the same
// database as an offline migration connection, before domain migrations run.
func AllowMetadataRead(ctx context.Context, db *sql.DB, role string) error {
	identifier := pgx.Identifier{role}.Sanitize()
	for _, q := range []string{"GRANT USAGE ON SCHEMA ding_meta TO " + identifier, "GRANT SELECT ON ding_meta.owner TO " + identifier} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return SafeError("grant metadata probe", err)
		}
	}
	return nil
}
