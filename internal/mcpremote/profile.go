// Package mcpremote connects to a deliberately configured hosted MCP resource.
// Its private OAuth profile is independent of administrator and CLI logins.
package mcpremote

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/oauth2"
)

type Profile struct {
	URL       string           `json:"url"`
	Issuer    string           `json:"issuer"`
	ClientID  string           `json:"clientID"`
	Workspace string           `json:"workspace"`
	Instance  string           `json:"instance"`
	Scopes    []string         `json:"scopes"`
	TokenURL  string           `json:"tokenURL"`
	AuthStyle oauth2.AuthStyle `json:"authStyle"`
	Token     *oauth2.Token    `json:"token"`
}

func (Profile) String() string     { return "Ding cloud OAuth profile (credentials redacted)" }
func (p Profile) GoString() string { return p.String() }

var id = regexp.MustCompile(`^[a-f0-9]{64}$`)

func secureURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func (p Profile) ValidateSetup() error {
	u, err := url.Parse(p.URL)
	if err != nil || !secureURL(p.URL) || u.Path != "/mcp" || !secureURL(p.Issuer) || p.ClientID == "" || len(p.ClientID) > 512 || !id.MatchString(p.Workspace) {
		return mcpconfig.ErrConfig
	}
	if !slices.Contains(p.Scopes, "ding:inspect") || len(p.Scopes) > 5 {
		return mcpconfig.ErrConfig
	}
	for _, scope := range p.Scopes {
		if !slices.Contains([]string{"ding:inspect", "ding:preview", "ding:manage", "ding:retry", "ding:move"}, scope) {
			return mcpconfig.ErrConfig
		}
	}
	return nil
}

func (p Profile) Validate() error {
	if err := p.ValidateSetup(); err != nil {
		return err
	}
	if p.Instance == "" || len(p.Instance) > 256 || !secureURL(p.TokenURL) || p.Token == nil || p.Token.AccessToken == "" || !strings.EqualFold(p.Token.TokenType, "Bearer") || p.Token.Expiry.IsZero() {
		return mcpconfig.ErrConfig
	}
	return nil
}

func Load(path string) (Profile, error) {
	var p Profile
	if err := mcpconfig.ReadPrivateJSON(path, &p, true); err != nil {
		return p, err
	}
	return p, p.Validate()
}
