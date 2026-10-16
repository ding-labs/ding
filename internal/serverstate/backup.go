package state

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"os"
)

func (d *DB) Backup(ctx context.Context, path string) error {
	if d.postgres {
		return fmt.Errorf("PostgreSQL requires an instance-level database backup")
	}
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
	if !d.postgres {
		var integrity string
		if err := d.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
			return err
		}
		if integrity != "ok" {
			return fmt.Errorf("restored control database failed integrity check")
		}
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []statement{
		{sqlite: "DELETE FROM webhook_connections", postgres: "DELETE FROM ding_control.webhook_connections"},
		{sqlite: "DELETE FROM service_grants", postgres: "DELETE FROM ding_control.service_grants"},
		{sqlite: "DELETE FROM activations", postgres: "DELETE FROM ding_control.activations"},
		{sqlite: "DELETE FROM sessions", postgres: "DELETE FROM ding_control.sessions"},
		{sqlite: "DELETE FROM logins", postgres: "DELETE FROM ding_control.logins"},
		{sqlite: "DELETE FROM devices", postgres: "DELETE FROM ding_control.devices"},
		{sqlite: "DELETE FROM mcp_bindings", postgres: "DELETE FROM ding_control.mcp_bindings"},
		{sqlite: "DELETE FROM handoff_proofs", postgres: "DELETE FROM ding_control.handoff_proofs"},
	} {
		if _, err = tx.ExecContext(ctx, tx.query(q)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
