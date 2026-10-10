package state

import (
	"context"
	"crypto/rand"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"testing"
)

func TestModelBindingNeedsExactWorkspaceAndClient(t *testing.T) {
	ctx := context.Background()
	db, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	key := make([]byte, 32)
	rand.Read(key)
	v, _ := NewVault(db, key)
	c := mcpconfig.Connection{DaemonURL: "https://ding.example", Token: "ding_mcp_" + ID(), GrantID: ID()}
	if e := v.BindMCP(ctx, a.ID, "client-a", c); e != nil {
		t.Fatal(e)
	}
	for _, pair := range [][2]string{{b.ID, "client-a"}, {a.ID, "client-b"}} {
		if _, e := v.MCP(ctx, pair[0], pair[1]); e == nil {
			t.Fatal("connection crossed boundary")
		}
	}
	if e := v.BindMCP(ctx, a.ID, "client-a", c); e == nil {
		t.Fatal("grant replaced without disconnect")
	}
	got, e := v.MCP(ctx, a.ID, "client-a")
	if e != nil || got.Token != c.Token {
		t.Fatal(e)
	}
	if e := db.UnbindMCP(ctx, a.ID, "client-a"); e != nil {
		t.Fatal(e)
	}
	if _, e := v.MCP(ctx, a.ID, "client-a"); e == nil {
		t.Fatal("revoked binding resurrected")
	}
}
