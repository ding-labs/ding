package state

import (
	"bytes"
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

func TestPostgresVaultAndClientConsentBoundaries(t *testing.T) {
	db := postgresTestDB(t)
	ctx := context.Background()
	a, err := db.Enroll(ctx, "issuer", "alice", 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.Enroll(ctx, "issuer", "bob", 2)
	if err != nil {
		t.Fatal(err)
	}
	vault, err := NewVault(db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	for workspace, value := range map[string]string{a.ID: "synthetic-alpha", b.ID: "synthetic-beta"} {
		if err = vault.Put(ctx, workspace, "TOKEN", value); err != nil {
			t.Fatal(err)
		}
		if got, err := vault.Get(ctx, workspace, "TOKEN"); err != nil || got != value {
			t.Fatal("isolated secret lookup failed", err)
		}
	}
	var encrypted []byte
	if err = db.sql.QueryRow("SELECT ciphertext FROM ding_control.secrets WHERE workspace=$1", a.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("synthetic-alpha")) {
		t.Fatal("plaintext stored")
	}
	if _, err = db.sql.Exec("UPDATE ding_control.secrets SET ciphertext=$1 WHERE workspace=$2", encrypted, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = vault.Get(ctx, b.ID, "TOKEN"); err == nil {
		t.Fatal("cross-workspace ciphertext accepted")
	}
	c := mcpconfig.Connection{DaemonURL: "https://ding.example", Token: "ding_mcp_" + ID(), GrantID: ID()}
	if err = vault.BindMCP(ctx, a.User, a.ID, "client-a", c); err != nil {
		t.Fatal(err)
	}
	if _, err = vault.MCP(ctx, b.User, a.ID, "client-a"); err == nil {
		t.Fatal("connection crossed user boundary")
	}
	if _, err = vault.MCP(ctx, a.User, b.ID, "client-a"); err == nil {
		t.Fatal("connection crossed workspace boundary")
	}
	if _, err = vault.MCP(ctx, a.User, a.ID, "client-b"); err == nil {
		t.Fatal("connection crossed client boundary")
	}
	if refs, err := db.MCPWorkspaces(ctx, a.User, "client-a"); err != nil || len(refs) != 1 || refs[0].Workspace != a.ID {
		t.Fatal("consent discovery mismatch", err)
	}
	if got, err := vault.MCP(ctx, a.User, a.ID, "client-a"); err != nil || got.Token != c.Token {
		t.Fatal("encrypted connection mismatch", err)
	}
	if err = db.UnbindMCP(ctx, a.User, a.ID, "client-a"); err != nil {
		t.Fatal(err)
	}
	if _, err = vault.MCP(ctx, a.User, a.ID, "client-a"); err == nil {
		t.Fatal("revoked connection resurrected")
	}
}
