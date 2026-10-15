package cloud

import (
	"context"
	"fmt"

	"github.com/ding-labs/ding/internal/database"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

// MigrateDatabase is offline and uses an explicit environment reference for the
// separate migration role. Server startup never acquires migration credentials.
func MigrateDatabase(ctx context.Context, c Config, adminEnv string) error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if c.Database.Effective().Backend == "sqlite" {
		if adminEnv != "" {
			return fmt.Errorf("database-admin-env requires PostgreSQL backend selection")
		}
		storage, err := OpenServerStorage(ctx, c.DataDir, c.Database)
		if err != nil {
			return err
		}
		defer storage.Close()
		workspaces, err := storage.Control.Workspaces(ctx)
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			s, err := storage.workspaces.Open(ctx, w.ID)
			if err != nil {
				return err
			}
			if err = s.Close(); err != nil {
				return err
			}
		}
		return nil
	}
	if adminEnv == "" {
		return fmt.Errorf("PostgreSQL migration requires --database-admin-env naming the separate migration DSN reference")
	}
	appRole, err := c.Database.Role(nil)
	if err != nil {
		return err
	}
	migrationConfig := c.Database
	migrationConfig.DSNRef = &database.SecretRef{Env: adminEnv}
	migrationConfig.MaxOpenConns = 1
	dedicated, err := database.OpenPostgres(ctx, migrationConfig, nil)
	if err != nil {
		return err
	}
	owner, err := database.TakeMigrationOwnership(ctx, dedicated)
	if err != nil {
		return err
	}
	defer owner.Close()
	ctx = database.WithOwnership(owner.Context(), owner)
	migration, err := database.OpenPostgres(ctx, migrationConfig, nil)
	if err != nil {
		return err
	}
	defer migration.Close()
	if err = database.AllowMetadataRead(ctx, migration, appRole); err != nil {
		return err
	}
	app, err := database.OpenPostgres(ctx, c.Database, nil)
	if err != nil {
		return err
	}
	defer app.Close()
	probe, err := app.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	err = owner.Fence(ctx, probe, false)
	probe.Rollback()
	if err != nil {
		return fmt.Errorf("application and migration DSN references must identify the same PostgreSQL database")
	}
	if err = state.MigratePostgres(ctx, migration, appRole); err != nil {
		return err
	}
	return store.MigratePostgres(ctx, migration, appRole)
}
