package pluginpackage

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cloudOptions(t *testing.T, client OAuthClient) Options {
	t.Helper()
	cfg := CloudConfig{Client: client, SupportURL: "https://ding.example/support", PrivacyPolicyURL: "https://ding.example/privacy", TermsOfServiceURL: "https://ding.example/terms"}
	config := filepath.Join(t.TempDir(), "publisher.json")
	if err := writeJSON(config, cfg); err != nil {
		t.Fatal(err)
	}
	return Options{Root: "../..", Mode: "cloud-chatgpt", CloudConfig: config, Output: filepath.Join(t.TempDir(), "package"), Version: "0.1.0"}
}

func TestCloudPackages(t *testing.T) {
	for _, client := range []OAuthClient{
		{Mode: "provided", ClientID: "registered-chatgpt-client", TokenEndpointAuthMethod: "client_secret_basic"},
		{Mode: "provided", ClientID: "registered-public-client", TokenEndpointAuthMethod: "none"},
		{Mode: "cimd"}, {Mode: "dcr"},
	} {
		t.Run(client.Mode+client.TokenEndpointAuthMethod, func(t *testing.T) {
			archive, err := Build(cloudOptions(t, client))
			if err != nil {
				t.Fatal(err)
			}
			r, err := zip.OpenReader(archive)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			read := func(name string) []byte {
				t.Helper()
				f, err := r.Open(name)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				raw, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				return raw
			}
			var config struct {
				Servers map[string]struct {
					Type       string `json:"type"`
					URL        string `json:"url"`
					Extensions map[string]struct {
						Auth struct {
							Type   string      `json:"type"`
							Client OAuthClient `json:"client"`
							Scopes []string    `json:"baseScopes"`
						} `json:"auth"`
					} `json:"extensions"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(read("mcp.json"), &config); err != nil {
				t.Fatal(err)
			}
			server := config.Servers["ding"]
			auth := server.Extensions["com.openai"].Auth
			if server.Type != "streamable-http" || server.URL != cloudEndpoint || auth.Type != "oauth" || auth.Client != client || strings.Join(auth.Scopes, " ") != "ding:inspect ding:preview ding:manage ding:retry" {
				t.Fatalf("incorrect cloud connection: %+v", server)
			}
			var manifest map[string]any
			if err := json.Unmarshal(read("plugin.json"), &manifest); err != nil {
				t.Fatal(err)
			}
			listing := manifest["extensions"].(map[string]any)["com.openai"].(map[string]any)["interface"].(map[string]any)
			if listing["privacyPolicyURL"] != "https://ding.example/privacy" || listing["supportURL"] != "https://ding.example/support" || listing["termsOfServiceURL"] != "https://ding.example/terms" || len([]rune(listing["shortDescription"].(string))) > 30 {
				t.Fatal(listing)
			}
			// Inspect the assembled ZIP, not just the source template: no local setup
			// workflow or local-only manifest example may leak into the cloud package.
			for _, name := range []string{"skills/ding-setup/SKILL.md", "skills/ding-watch/SKILL.md", "skills/ding-watch/references/manifest.md", "README.md"} {
				body := string(read(name))
				for _, unsupported := range []string{"ding-mcp setup", "Ding Setup", "ding_prepare_transfer", "type: console", "every: 5s"} {
					if strings.Contains(body, unsupported) {
						t.Fatalf("%s contains unavailable workflow %s", name, unsupported)
					}
				}
			}
			read("privacy.md")
			read("assets/icon.svg")
		})
	}
}

func TestInvalidCloudConfigurationCreatesNoArtifact(t *testing.T) {
	for name, mutate := range map[string]func(*Options){
		"missing-config": func(o *Options) { o.CloudConfig = "" },
		"other-endpoint": func(o *Options) { o.Endpoint = "https://other.example/mcp" },
		"secret": func(o *Options) {
			_ = os.WriteFile(o.CloudConfig, []byte(`{"client":{"mode":"provided","clientId":"id","clientSecret":"do-not-package"}}`), 0600)
		},
		"bad-client-mode": func(o *Options) {
			var cfg CloudConfig
			b, _ := os.ReadFile(o.CloudConfig)
			_ = json.Unmarshal(b, &cfg)
			cfg.Client.Mode = "cimd"
			_ = writeJSON(o.CloudConfig, cfg)
		},
		"missing-privacy": func(o *Options) {
			var cfg CloudConfig
			b, _ := os.ReadFile(o.CloudConfig)
			_ = json.Unmarshal(b, &cfg)
			cfg.PrivacyPolicyURL = ""
			_ = writeJSON(o.CloudConfig, cfg)
		},
		"credential-url": func(o *Options) {
			var cfg CloudConfig
			b, _ := os.ReadFile(o.CloudConfig)
			_ = json.Unmarshal(b, &cfg)
			cfg.SupportURL = "https://user:secret@example.com/help"
			_ = writeJSON(o.CloudConfig, cfg)
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := cloudOptions(t, OAuthClient{Mode: "provided", ClientID: "client", TokenEndpointAuthMethod: "none"})
			mutate(&o)
			if _, err := Build(o); err == nil {
				t.Fatal("accepted invalid publisher configuration")
			}
			if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
				t.Fatal("artifact created before validation")
			}
		})
	}
}
