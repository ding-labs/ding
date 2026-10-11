package state

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// ConfigureDataKey detects accidental key replacement across Railway restarts
// and restores. Only the fingerprint of a high-entropy external key is stored.
func (d *DB) ConfigureDataKey(ctx context.Context, key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("data key must contain 32 bytes")
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(append([]byte("ding.cloud.key.v1/"), key...)))
	if _, err := d.sql.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('data_key',?) ON CONFLICT(key) DO NOTHING", fingerprint); err != nil {
		return err
	}
	var saved string
	if err := d.sql.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='data_key'").Scan(&saved); err != nil {
		return err
	}
	if saved != fingerprint {
		return fmt.Errorf("data key differs from initialized state; restore the original external key")
	}
	return nil
}

func (d *DB) ConfigureHosting(ctx context.Context, hosting string) error {
	if hosting != "cloud" && hosting != "self-hosted" {
		return fmt.Errorf("invalid hosting profile")
	}
	if _, err := d.sql.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('hosting',?) ON CONFLICT(key) DO NOTHING", hosting); err != nil {
		return err
	}
	var saved string
	if err := d.sql.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='hosting'").Scan(&saved); err != nil {
		return err
	}
	if saved != hosting {
		return fmt.Errorf("hosting profile differs from the initialized instance")
	}
	return nil
}

// ConfigureLoginMethods pins the instance's admission methods independently of
// its execution policy. A configuration typo must not open a new login path.
func (d *DB) ConfigureLoginMethods(ctx context.Context, methods []string) error {
	sorted := append([]string(nil), methods...)
	sort.Strings(sorted)
	value := strings.Join(sorted, ",")
	if value != "password" && value != "oidc" && value != "oidc,password" {
		return fmt.Errorf("invalid login methods")
	}
	if _, err := d.sql.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('login_methods',?) ON CONFLICT(key) DO NOTHING", value); err != nil {
		return err
	}
	var saved string
	if err := d.sql.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='login_methods'").Scan(&saved); err != nil {
		return err
	}
	if saved != value {
		return fmt.Errorf("login methods differ from the initialized instance")
	}
	return nil
}
