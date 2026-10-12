package mcpserver

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestOAuthCapabilitiesCannotOverstateDaemonGrant(t *testing.T) {
	grant := map[string]any{"scopes": []any{"inspect", "manage"}}
	token := &auth.TokenInfo{Scopes: []string{"ding:inspect", "ding:retry"}}
	for _, caps := range []map[string]any{
		{"grant": grant, "canManageDestinations": true},
		{"grant": grant, "effectiveScopes": []any{}, "canManageDestinations": true},
	} {
		got := tokenCapabilities(caps, token).(map[string]any)
		want := []string{"inspect"}
		if _, exists := caps["effectiveScopes"]; exists {
			want = []string{}
		}
		if !reflect.DeepEqual(got["effectiveScopes"], want) || got["canManageDestinations"] != false {
			t.Fatal(got)
		}
		if caps["canManageDestinations"] != true || !reflect.DeepEqual(grant["scopes"], []any{"inspect", "manage"}) {
			t.Fatal("mutated upstream capabilities")
		}
	}
}
