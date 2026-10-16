package railway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/identity"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

// configureMCP is opt-in. The generic profile keeps admission closed; AuthKit
// requires an explicit capacity and uses hosted sign-in exclusively.
func configureMCP(c *cloud.Config, get func(string) string) error {
	c.AuthKitAPIKey = get("DING_AUTHKIT_API_KEY")
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
	if raw := get("DING_AUTHKIT_JSON"); raw != "" {
		for _, key := range []string{"DING_OIDC_JSON", "DING_MCP_JSON", "DING_MCP_BRIDGE_CLIENTS"} {
			if get(key) != "" {
				return fmt.Errorf("DING_AUTHKIT_JSON cannot be combined with legacy OAuth configuration")
			}
		}
		var settings struct {
			Issuer                     string   `json:"issuer"`
			BrowserClientID            string   `json:"browserClientId"`
			ChatGPTClientID            string   `json:"chatgptClientId"`
			AdditionalChatGPTClientIDs []string `json:"additionalChatgptClientIds"`
			EnrollmentLimit            *int     `json:"enrollmentLimit"`
		}
		if err := decode(raw, &settings); err != nil {
			return err
		}
		issuer, err := url.Parse(settings.Issuer)
		if err != nil || issuer.Path != "" {
			return fmt.Errorf("AuthKit issuer must be an HTTPS origin without a trailing slash")
		}
		if settings.EnrollmentLimit == nil || settings.BrowserClientID == "" || settings.ChatGPTClientID == "" || settings.BrowserClientID == settings.ChatGPTClientID {
			return fmt.Errorf("AuthKit requires separate browser and ChatGPT public clients and an explicit enrollmentLimit")
		}
		// Public Connect clients use PKCE. No WorkOS API key or OAuth secret is
		// required to verify JWTs or complete the browser authorization flow.
		c.Identity = identity.Config{Issuer: settings.Issuer, ClientID: settings.BrowserClientID}
		c.LoginMethods = []string{"oidc"}
		c.MCP = &mcpconfig.HTTP{Issuer: settings.Issuer, JWKSURI: settings.Issuer + "/oauth2/jwks", Audience: strings.TrimRight(c.PublicURL, "/") + "/mcp", Algorithm: "RS256"}
		c.AuthKitClients = append([]string{settings.ChatGPTClientID}, settings.AdditionalChatGPTClientIDs...)
		c.EnrollmentLimit = settings.EnrollmentLimit
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
	return strings.HasPrefix(name, "DING_DATABASE_") || name == "DING_DATA_KEY_BASE64" || strings.HasPrefix(name, "DING_BOOTSTRAP_") || strings.HasPrefix(name, "DING_OIDC_") || strings.HasPrefix(name, "DING_MCP_") || strings.HasPrefix(name, "DING_AUTHKIT_")
}
