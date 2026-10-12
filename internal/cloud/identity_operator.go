package cloud

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
)

func EnableOIDC(ctx context.Context, c Config, link state.IdentityLink) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !slices.Contains(c.LoginMethods, "password") || !slices.Contains(c.LoginMethods, "oidc") || c.Identity.Issuer != link.Issuer {
		return fmt.Errorf("review a password+oidc configuration with the exact verified issuer")
	}
	if err := mcpconfig.CheckPrivate(filepath.Join(c.DataDir, "control.db")); err != nil {
		return err
	}
	key, err := loadKey(&c)
	if err != nil {
		return err
	}
	db, err := state.Open(ctx, c.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	if c.Deployment == "railway" {
		if err := db.ConfigureDataKey(ctx, key); err != nil {
			return err
		}
	}
	if err := db.ConfigureHosting(ctx, c.Hosting); err != nil {
		return err
	}
	return db.EnableOIDC(ctx, link)
}
