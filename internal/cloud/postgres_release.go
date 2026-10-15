package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/cloudbackup"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

// ReleasePostgresRestore releases only a fully authenticated, quarantined
// restore. The caller must first verify that the source and transferred runners
// are stopped. Watches stay paused and credentials stay revoked.
func ReleasePostgresRestore(ctx context.Context, c Config) error {
	if c.Database.Effective().Backend != "postgres" {
		return fmt.Errorf("PostgreSQL backend required")
	}
	marker := filepath.Join(c.DataDir, cloudbackup.Quarantine)
	var status postgresRestoreMarker
	if err := mcpconfig.ReadPrivateJSON(marker, &status, true); err != nil {
		return err
	}
	if status.Format != "ding-postgres/v1" || !status.Complete {
		return fmt.Errorf("restore is incomplete; review or retry it before release")
	}
	s, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx = s.Context()
	key, err := loadKey(&c)
	if err != nil {
		return err
	}
	if err = s.Control.ConfigureDataKey(ctx, key); err != nil {
		return err
	}
	// Reapply after offline recovery operations, before clearing the DB flag.
	if err = quarantinePostgresState(ctx, s); err != nil {
		return err
	}
	workspaces, err := s.Control.Workspaces(ctx)
	if err != nil {
		return err
	}
	for _, w := range workspaces {
		tenant, err := s.workspaces.Open(ctx, w.ID)
		if err != nil {
			return err
		}
		err = tenant.ReleaseRestoredHolds(ctx)
		closed := tenant.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	if err = database.SetQuarantine(ctx, s.pool, s.owner, false); err != nil {
		return err
	}
	return os.Remove(marker)
}
