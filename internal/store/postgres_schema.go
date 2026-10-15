package store

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/ding-labs/ding/internal/database"
)

// PostgreSQL migrations have their own version stream. SQLite user_version and
// existing on-disk migrations are independent.
const PostgresSchemaVersion = 3

//go:embed postgres/001_records.sql
var postgresRecordsSchema string

//go:embed postgres/002_state.sql
var postgresStateSchema string

//go:embed postgres/003_events.sql
var postgresEventsSchema string

func MigratePostgres(ctx context.Context, migrationDB *sql.DB, appRole string) error {
	return database.Migrate(ctx, migrationDB, "runtime", []string{postgresRecordsSchema, postgresStateSchema, postgresEventsSchema}, appRole)
}
