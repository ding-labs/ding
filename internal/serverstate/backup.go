package state

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"os"
)

func (d *DB) Backup(ctx context.Context, path string) error {
	f, err := mcpconfig.CreatePrivate(path)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := d.sql.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return err
	}
	f, err = os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (d *DB) QuarantineRestore(ctx context.Context) error {
	var integrity string
	if err := d.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("restored control database failed integrity check")
	}
	_, err := d.sql.ExecContext(ctx, "DELETE FROM sessions; DELETE FROM logins; DELETE FROM devices; DELETE FROM mcp_bindings; DELETE FROM handoff_proofs;")
	return err
}
