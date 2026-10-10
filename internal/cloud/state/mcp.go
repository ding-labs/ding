package state

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

func (d *DB) IdentityAccount(ctx context.Context, issuer, subject string) (Account, error) {
	return scanAccount(d.sql.QueryRowContext(ctx, "SELECT id,issuer,subject,created_at FROM accounts WHERE issuer=? AND subject=? AND deleting=0", issuer, subject))
}

// MCP connections are separate from source credentials and never appear in the
// secret-name inventory. Both tenant and signed OAuth client ID bind the cipher.
func (v *Vault) BindMCP(ctx context.Context, account, client string, c mcpconfig.Connection) error {
	if client == "" || len(client) > 512 {
		return fmt.Errorf("invalid OAuth client")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	aead, err := v.aead("mcp/" + account)
	if err != nil {
		return err
	}
	cipher := aead.Seal(nil, nil, raw, []byte(account+"\x00"+client))
	tx, err := v.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM mcp_bindings WHERE account=?", account).Scan(&n); err != nil {
		return err
	}
	if n >= 10 {
		return fmt.Errorf("disconnect an existing model client first")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO mcp_bindings(account,client,ciphertext) VALUES(?,?,?)", account, client, cipher)
	if err != nil {
		return fmt.Errorf("client already connected or workspace unavailable")
	}
	return tx.Commit()
}
func (v *Vault) MCP(ctx context.Context, account, client string) (mcpconfig.Connection, error) {
	var c mcpconfig.Connection
	var cipher []byte
	if err := v.db.sql.QueryRowContext(ctx, "SELECT ciphertext FROM mcp_bindings WHERE account=? AND client=?", account, client).Scan(&cipher); err != nil {
		return c, err
	}
	aead, err := v.aead("mcp/" + account)
	if err != nil {
		return c, err
	}
	plain, err := aead.Open(nil, nil, cipher, []byte(account+"\x00"+client))
	if err != nil {
		return c, fmt.Errorf("model connection unavailable")
	}
	if err := json.Unmarshal(plain, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func (d *DB) MCPClients(ctx context.Context, account string) ([]string, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT client FROM mcp_bindings WHERE account=? ORDER BY client", account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var client string
		if err := rows.Scan(&client); err != nil {
			return nil, err
		}
		result = append(result, client)
	}
	return result, rows.Err()
}
func (d *DB) UnbindMCP(ctx context.Context, account, client string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM mcp_bindings WHERE account=? AND client=?", account, client)
	return err
}
