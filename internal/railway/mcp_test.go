package railway

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestOptionalMCPKeepsCloudPolicyAndClosedSignup(t *testing.T) {
	env := environment()
	env["DING_OIDC_JSON"] = `{"issuer":"https://auth.example","clientId":"browser","clientSecret":"private-value"}`
	env["DING_MCP_JSON"] = `{"issuer":"https://auth.example","jwks_uri":"https://auth.example/keys","audience":"https://cloud.ding.ing/mcp"}`
	env["DING_MCP_BRIDGE_CLIENTS"] = "native-bridge"
	s, _, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil || s.Config.MCP == nil || len(s.Config.LoginMethods) != 2 || s.Config.Hosting != "cloud" || *s.Config.EnrollmentLimit != 0 || len(s.Config.MCPBridgeClients) != 1 {
		t.Fatal("incorrect OAuth profile", err)
	}
	for _, key := range []string{"DING_OIDC_JSON", "DING_MCP_JSON", "DING_BOOTSTRAP_PASSWORD", "DING_DATA_KEY_BASE64"} {
		if !SecretEnvironment(key) {
			t.Fatal("secret environment retained", key)
		}
	}
	if SecretEnvironment("PORT") {
		t.Fatal("lost ordinary environment")
	}
	for _, change := range []map[string]string{
		{"DING_OIDC_JSON": ""},
		{"DING_OIDC_JSON": `{"issuer":"http://auth.example","clientId":"browser","clientSecret":"private-value"}`},
		{"DING_MCP_JSON": `{"issuer":"https://other.example","jwks_uri":"https://auth.example/keys","audience":"https://cloud.ding.ing/mcp"}`},
		{"DING_MCP_JSON": `{"issuer":"https://auth.example","jwks_uri":"https://auth.example/keys","audience":"https://cloud.ding.ing"}`},
		{"DING_MCP_JSON": ""},
	} {
		copy := map[string]string{}
		for k, v := range env {
			copy[k] = v
		}
		for k, v := range change {
			copy[k] = v
		}
		_, _, _, err := FromEnv(func(k string) string { return copy[k] })
		if err == nil {
			t.Fatal("invalid OAuth profile accepted")
		}
		if strings.Contains(err.Error(), "private-value") {
			t.Fatal("secret appeared in diagnostic")
		}
	}
}

func TestAuthKitAdditionalClients(t *testing.T) {
	for name, clients := range map[string][]string{
		"private-test":      {"private-test"},
		"duplicate-primary": {"chatgpt"},
		"duplicate-extra":   {"test", "test"},
		"browser-client":    {"console"},
		"empty":             {""},
		"whitespace":        {" private-test"},
		"too-many":          {"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"issuer": "https://sample.authkit.app", "browserClientId": "console", "chatgptClientId": "chatgpt", "additionalChatgptClientIds": clients, "enrollmentLimit": 10})
			if err != nil {
				t.Fatal(err)
			}
			env := environment()
			env["DING_AUTHKIT_JSON"] = string(raw)
			s, _, _, err := FromEnv(func(k string) string { return env[k] })
			if name != "private-test" {
				if err == nil {
					t.Fatal("invalid client allowlist accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(s.Config.AuthKitClients, []string{"chatgpt", "private-test"}) {
				t.Fatal("private client replaced public client")
			}
		})
	}
}

func TestAuthKitProfile(t *testing.T) {
	env := environment()
	const valid = `{"issuer":"https://sample.authkit.app","browserClientId":"console","chatgptClientId":"chatgpt","enrollmentLimit":10}`
	env["DING_AUTHKIT_JSON"] = valid
	settings, _, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	c := settings.Config
	if len(c.LoginMethods) != 1 || c.LoginMethods[0] != "oidc" {
		t.Fatal("AuthKit must not enable instance password login")
	}
	if c.MCP.JWKSURI != "https://sample.authkit.app/oauth2/jwks" || c.MCP.Audience != "https://cloud.ding.ing/mcp" || c.AuthKitClients[0] != "chatgpt" || c.Identity.ClientID != "console" || c.Identity.ClientSecret != "" || *c.EnrollmentLimit != 10 {
		t.Fatal("incorrect AuthKit profile")
	}
	if !SecretEnvironment("DING_AUTHKIT_JSON") {
		t.Fatal("AuthKit config leaked to child processes")
	}
	for _, raw := range []string{
		strings.Replace(valid, `,"enrollmentLimit":10`, "", 1),
		strings.Replace(valid, `"chatgpt"`, `"console"`, 1),
		strings.Replace(valid, `:10`, `:1001`, 1),
		strings.Replace(valid, `https://`, `http://`, 1),
		strings.Replace(valid, `"chatgpt"`, `""`, 1),
		valid + `{}`,
		strings.Replace(valid, `"enrollmentLimit"`, `"unknownSecret"`, 1),
	} {
		env["DING_AUTHKIT_JSON"] = raw
		if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
			t.Fatal("invalid AuthKit configuration accepted", raw)
		}
	}
	env["DING_AUTHKIT_JSON"] = valid
	env["DING_OIDC_JSON"] = `{}`
	if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("ambiguous provider configuration accepted")
	}
}

func TestOpenAIChallengeProfile(t *testing.T) {
	env := environment()
	env["DING_OPENAI_APPS_CHALLENGE"] = "exact-portal-token"
	s, _, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil || s.Config.OpenAIAppsChallenge != "exact-portal-token" {
		t.Fatal("domain challenge lost", err)
	}
	env["DING_OPENAI_APPS_CHALLENGE"] += "\n"
	if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("malformed challenge accepted")
	}
}
