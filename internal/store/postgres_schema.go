package store

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/ding-labs/ding/internal/database"
)

// PostgreSQL migrations have their own version stream. SQLite user_version and
// existing on-disk migrations are independent.
const PostgresSchemaVersion = 10

//go:embed postgres/001_records.sql
var postgresRecordsSchema string

//go:embed postgres/002_state.sql
var postgresStateSchema string

//go:embed postgres/003_events.sql
var postgresEventsSchema string

//go:embed postgres/004_push.sql
var postgresPushSchema string

//go:embed postgres/005_integrations.sql
var postgresIntegrationsSchema string

//go:embed postgres/006_storage.sql
var postgresStorageSchema string

//go:embed postgres/007_canonical_accounting.sql
var postgresCanonicalAccountingSchema string

//go:embed postgres/008_delivery_streams.sql
var postgresDeliveryStreamsSchema string

//go:embed postgres/009_entity_pins.sql
var postgresEntityPinsSchema string

//go:embed postgres/010_retention_indexes.sql
var postgresRetentionIndexesSchema string

func MigratePostgres(ctx context.Context, migrationDB *sql.DB, appRole string) error {
	return database.Migrate(ctx, migrationDB, "runtime", []string{postgresRecordsSchema, postgresStateSchema, postgresEventsSchema, postgresPushSchema, postgresIntegrationsSchema, postgresStorageSchema, postgresCanonicalAccountingSchema, postgresDeliveryStreamsSchema, postgresEntityPinsSchema, postgresRetentionIndexesSchema}, appRole)
}
