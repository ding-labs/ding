package state

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

// MCPWorkspaces returns only this user's membership-backed client bindings.
// The caller also checks the underlying grant's current expiry and revocation.
func (d *DB) MCPWorkspaces(ctx context.Context, user, client string) ([]Membership, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT w.id,w.name,m.role FROM mcp_bindings b JOIN memberships m ON m.user=b.user AND m.workspace=b.workspace JOIN workspaces w ON w.id=b.workspace JOIN users u ON u.id=b.user WHERE b.user=? AND b.client=? AND w.deleting=0 AND u.disabled=0 ORDER BY w.id`, user, client)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var member Membership
		if err := rows.Scan(&member.Workspace, &member.Name, &member.Role); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

// ResolveMCPWorkspace never borrows the browser's selection. With no explicit
// workspace, only an unambiguous approved connection may be used.
func (d *DB) ResolveMCPWorkspace(ctx context.Context, user, client, selected string) (string, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT b.workspace FROM mcp_bindings b JOIN memberships m ON m.user=b.user AND m.workspace=b.workspace JOIN workspaces w ON w.id=b.workspace WHERE b.user=? AND b.client=? AND w.deleting=0 AND (?='' OR b.workspace=?) LIMIT 2`, user, client, selected, selected)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var workspace string
	count := 0
	for rows.Next() {
		if err := rows.Scan(&workspace); err != nil {
			return "", err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if count != 1 {
		return "", fmt.Errorf("select one approved workspace")
	}
	return workspace, nil
}

// MCP connections are separate from source credentials and never appear in the
// secret-name inventory. Both tenant and signed OAuth client ID bind the cipher.
func (v *Vault) BindMCP(ctx context.Context, user, workspace, client string, c mcpconfig.Connection) error {
	if _, err := v.db.Principal(ctx, user, workspace, "browser", nil); err != nil {
		return fmt.Errorf("membership required")
	}
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
	aead, err := v.aead("mcp/" + workspace)
	if err != nil {
		return err
	}
	cipher := aead.Seal(nil, nil, raw, []byte(user+"\x00"+workspace+"\x00"+client))
	tx, err := v.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM mcp_bindings WHERE user=? AND workspace=?", user, workspace).Scan(&n); err != nil {
		return err
	}
	if n >= 10 {
		return fmt.Errorf("disconnect an existing model client first")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO mcp_bindings(user,workspace,client,credential_hash,ciphertext) VALUES(?,?,?,?,?)", user, workspace, client, TokenHash(c.Token), cipher)
	if err != nil {
		return fmt.Errorf("client already connected or workspace unavailable")
	}
	return tx.Commit()
}
func (v *Vault) MCP(ctx context.Context, user, workspace, client string) (mcpconfig.Connection, error) {
	var c mcpconfig.Connection
	var cipher []byte
	if err := v.db.sql.QueryRowContext(ctx, "SELECT ciphertext FROM mcp_bindings WHERE user=? AND workspace=? AND client=?", user, workspace, client).Scan(&cipher); err != nil {
		return c, err
	}
	aead, err := v.aead("mcp/" + workspace)
	if err != nil {
		return c, err
	}
	plain, err := aead.Open(nil, nil, cipher, []byte(user+"\x00"+workspace+"\x00"+client))
	if err != nil {
		return c, fmt.Errorf("model connection unavailable")
	}
	if err := json.Unmarshal(plain, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func (d *DB) MCPClients(ctx context.Context, user, workspace string) ([]string, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT client FROM mcp_bindings WHERE user=? AND workspace=? ORDER BY client", user, workspace)
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
func (d *DB) UnbindMCP(ctx context.Context, user, workspace, client string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM mcp_bindings WHERE user=? AND workspace=? AND client=?", user, workspace, client)
	return err
}

// LocalAgentBinding resolves an explicitly exported self-hosted pairing file.
// Hosted public MCP continues to require its separate JWT identity boundary.
func (d *DB) LocalAgentBinding(ctx context.Context, token string) (user, workspace, client string, err error) {
	err = d.sql.QueryRowContext(ctx, "SELECT user,workspace,client FROM mcp_bindings WHERE credential_hash=? AND client LIKE 'local:%'", TokenHash(token)).Scan(&user, &workspace, &client)
	return
}
