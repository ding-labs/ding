package pluginpackage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const cloudEndpoint = "https://cloud.ding.ing/mcp"

// CloudConfig contains only public publisher settings. Secrets are supplied to
// the provider and ChatGPT portal, never to the package builder.
type CloudConfig struct {
	Client            OAuthClient `json:"client"`
	SupportURL        string      `json:"supportURL"`
	PrivacyPolicyURL  string      `json:"privacyPolicyURL"`
	TermsOfServiceURL string      `json:"termsOfServiceURL"`
}

type OAuthClient struct {
	Mode                    string `json:"mode"`
	ClientID                string `json:"clientId,omitempty"`
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`
}

func loadCloudConfig(path string) (CloudConfig, error) {
	var cfg CloudConfig
	f, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("cloud package requires a public publisher configuration: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() > 64<<10 {
		return cfg, errors.New("cloud publisher configuration must be at most 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, errors.New("invalid cloud publisher configuration; only public client and listing fields are accepted")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return cfg, errors.New("cloud publisher configuration must contain one JSON object")
	}
	switch cfg.Client.Mode {
	case "provided":
		if cfg.Client.ClientID == "" || strings.TrimSpace(cfg.Client.ClientID) != cfg.Client.ClientID || strings.ContainsAny(cfg.Client.ClientID, "\r\n\t") {
			return cfg, errors.New("provided OAuth client requires a clientId")
		}
		switch cfg.Client.TokenEndpointAuthMethod {
		case "none", "client_secret_basic", "client_secret_post", "private_key_jwt":
		default:
			return cfg, errors.New("provided OAuth client requires its tokenEndpointAuthMethod")
		}
	case "cimd", "dcr":
		if cfg.Client.ClientID != "" || cfg.Client.TokenEndpointAuthMethod != "" {
			return cfg, errors.New("CIMD and DCR must not include provided client settings")
		}
	default:
		return cfg, errors.New("choose the provider's supported OAuth client mode: provided, cimd or dcr")
	}
	for name, raw := range map[string]string{"supportURL": cfg.SupportURL, "privacyPolicyURL": cfg.PrivacyPolicyURL, "termsOfServiceURL": cfg.TermsOfServiceURL} {
		u, err := url.Parse(raw)
		if err != nil || len(raw) > 1024 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return cfg, fmt.Errorf("%s must be a credential-free public HTTPS listing URL", name)
		}
	}
	return cfg, nil
}
