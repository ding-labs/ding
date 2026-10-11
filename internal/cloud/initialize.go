package cloud

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
)

// Initialize is a host-only operation. It cannot run while the server owns state.
// The supplied encryption key must already exist outside the data directory.
func Initialize(ctx context.Context, c Config, username, name, password string, recover bool) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !slices.Contains(c.LoginMethods, "password") {
		return fmt.Errorf("built-in login must be enabled")
	}
	if err := ValidateServerDirectory(c.DataDir); err != nil {
		return err
	}
	if _, err := loadKey(&c); err != nil {
		return err
	}
	db, err := state.Open(ctx, c.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.ConfigureHosting(ctx, c.Hosting); err != nil {
		return err
	}
	if err := db.ConfigureLoginMethods(ctx, c.LoginMethods); err != nil {
		return err
	}
	if recover {
		return db.RecoverPassword(ctx, username, password)
	}
	_, err = db.Bootstrap(ctx, username, name, password)
	return err
}

func loadKey(c *Config) ([]byte, error) {
	f, err := mcpconfig.OpenPrivate(c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("open external encryption key: %w", err)
	}
	key, readErr := io.ReadAll(io.LimitReader(f, 33))
	closed := f.Close()
	if readErr != nil || closed != nil || len(key) != 32 {
		return nil, fmt.Errorf("keyFile must contain exactly 32 private bytes")
	}
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, err
	}
	c.DataDir, err = filepath.EvalSymlinks(c.DataDir)
	if err != nil {
		return nil, err
	}
	keyPath, err := filepath.EvalSymlinks(c.KeyFile)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(c.DataDir, keyPath)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("keep the encryption key outside the data and backup volume")
	}
	return key, nil
}
