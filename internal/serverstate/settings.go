package state

import (
	"context"
	"fmt"
)

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
