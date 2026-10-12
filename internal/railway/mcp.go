package railway

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

// configureMCP is opt-in. Admission remains closed and password login remains
// available. Existing state requires the offline enable-oidc operator step.
func configureMCP(c *cloud.Config, get func(string) string) error {
	decode := func(raw string, dest any) error {
		if len(raw) > 16<<10 {
			return fmt.Errorf("OAuth configuration exceeds 16 KiB")
		}
		d := json.NewDecoder(strings.NewReader(raw))
		d.DisallowUnknownFields()
		if d.Decode(dest) != nil {
			return fmt.Errorf("invalid private OAuth configuration JSON")
		}
		var trailing any
		if d.Decode(&trailing) != io.EOF {
			return fmt.Errorf("invalid trailing OAuth configuration")
		}
		return nil
	}
	if raw := get("DING_OIDC_JSON"); raw != "" {
		if err := decode(raw, &c.Identity); err != nil {
			return err
		}
		c.LoginMethods = []string{"password", "oidc"}
	}
	if raw := get("DING_MCP_JSON"); raw != "" {
		c.MCP = &mcpconfig.HTTP{}
		if err := decode(raw, c.MCP); err != nil {
			return err
		}
	}
	if raw := get("DING_MCP_BRIDGE_CLIENTS"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			c.MCPBridgeClients = append(c.MCPBridgeClients, strings.TrimSpace(id))
		}
	}
	return nil
}

func SecretEnvironment(name string) bool {
	return name == "DING_DATA_KEY_BASE64" || strings.HasPrefix(name, "DING_BOOTSTRAP_") || strings.HasPrefix(name, "DING_OIDC_") || strings.HasPrefix(name, "DING_MCP_")
}
