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
	return enableOIDC(ctx, c, link.Issuer, func(db *state.DB) error {
		return db.EnableOIDC(ctx, link)
	})
}

// EnableOIDCLogin activates the configured provider without assigning any
// existing password account to an external identity.
func EnableOIDCLogin(ctx context.Context, c Config) error {
	return enableOIDC(ctx, c, c.Identity.Issuer, func(db *state.DB) error {
		return db.EnableOIDCLogin(ctx, c.Identity.Issuer)
	})
}

func enableOIDC(ctx context.Context, c Config, issuer string, apply func(*state.DB) error) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !slices.Contains(c.LoginMethods, "password") || !slices.Contains(c.LoginMethods, "oidc") || c.Identity.Issuer != issuer {
		return fmt.Errorf("review a password+oidc configuration with the exact verified issuer")
	}
	if c.Database.Effective().Backend == "sqlite" {
		if err := mcpconfig.CheckPrivate(filepath.Join(c.DataDir, "control.db")); err != nil {
			return err
		}
	}
	key, err := loadKey(&c)
	if err != nil {
		return err
	}
	storage, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		return err
	}
	defer storage.Close()
	ctx = storage.Context()
	db := storage.Control
	if c.Deployment == "railway" || c.Database.Effective().Backend == "postgres" {
		if err := db.ConfigureDataKey(ctx, key); err != nil {
			return err
		}
	}
	if err := db.ConfigureHosting(ctx, c.Hosting); err != nil {
		return err
	}
	return apply(db)
}
