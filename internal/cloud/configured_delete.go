package cloud

import (
	"context"
	"fmt"
	"path/filepath"
)

func DeleteOfflineConfigured(ctx context.Context, c Config, id string) error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if c.Database.Effective().Backend == "sqlite" {
		return DeleteOffline(ctx, c.DataDir, id)
	}
	if !filepath.IsAbs(c.DataDir) || !accountID.MatchString(id) {
		return fmt.Errorf("absolute data directory and exact workspace ID required")
	}
	s, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx = s.Context()
	if err = s.Control.BeginWorkspaceDelete(ctx, id); err != nil {
		return err
	}
	if err = s.workspaces.Delete(ctx, id); err != nil {
		return err
	}
	return s.Control.FinishWorkspaceDelete(ctx, id)
}
