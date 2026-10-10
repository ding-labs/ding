package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExistingConnectionAndPrivateFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mcp.json")
	f, err := CreatePrivate(p)
	if err != nil {
		t.Fatal(err)
	}
	c := Connection{DaemonURL: "http://127.0.0.1:7676", Token: "ding_mcp_" + strings.Repeat("a", 64), GrantID: strings.Repeat("b", 64)}
	if err := json.NewEncoder(f).Encode(c); err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := LoadConnection(p)
	if err != nil || got != c {
		t.Fatalf("existing format: %v %v", got, err)
	}
	if _, err := CreatePrivate(p); !os.IsExist(err) {
		t.Fatal("existing pairing overwritten", err)
	}
	if runtime.GOOS != "windows" {
		link := p + ".link"
		if err := os.Symlink(p, link); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConnection(link); err == nil {
			t.Fatal("symlink accepted")
		}
		if err := os.Chmod(p, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConnection(p); err == nil {
			t.Fatal("broad permissions accepted")
		}
	}
}

func TestEndpointAndHTTPConfiguration(t *testing.T) {
	for _, value := range []string{"http://remote.example", "https://user:secret@example.com", "file:///tmp/ding", "https://example.com/path", "https://example.com?token=secret", "https://example.com#fragment"} {
		if _, err := Endpoint(value, false); err == nil {
			t.Fatal("unsafe endpoint", value)
		}
	}
	for _, value := range []string{"http://localhost:7676", "http://127.0.0.1:7676", "http://[::1]:7676", "https://ding.example"} {
		if _, err := Endpoint(value, false); err != nil {
			t.Fatal(value, err)
		}
	}
	h := HTTP{PublicURL: "https://ding.example", Issuer: "https://id.example", JWKSURI: "https://id.example/jwks", Audience: "https://ding.example/mcp", Subjects: map[string]string{"alice": filepath.Join(t.TempDir(), "alice.json")}, Algorithm: "Ed25519"}
	if err := h.Validate(); err != nil || h.Algorithm != "EdDSA" {
		t.Fatal(err)
	}
	h.Algorithm = "HS256"
	if h.Validate() == nil {
		t.Fatal("symmetric algorithm accepted")
	}
	h.Algorithm = "RS256"
	h.Subjects = nil
	if h.Validate() == nil {
		t.Fatal("no subjects accepted")
	}
	if got := configPath("linux", "/home/a", "/custom", ""); got != filepath.Join("/custom", "Ding", "mcp.json") {
		t.Fatal(got)
	}
	if got := configPath("darwin", "/home/a", "", ""); got != filepath.Join("/home/a", "Library", "Application Support", "Ding", "mcp.json") {
		t.Fatal(got)
	}
	if got := configPath("windows", "", "", `C:\Users\a\AppData\Local`); got != filepath.Join(`C:\Users\a\AppData\Local`, "Ding", "mcp.json") {
		t.Fatal(got)
	}
}
