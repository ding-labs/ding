package mcpremote

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/oauth2"
)

func testProfile() Profile {
	return Profile{URL: "https://cloud.example/mcp", Issuer: "https://auth.example/tenant", ClientID: "bridge", Workspace: strings.Repeat("a", 64), Instance: "instance", Scopes: []string{"ding:inspect"}, TokenURL: "https://auth.example/token", Token: &oauth2.Token{AccessToken: "secret", TokenType: "Bearer", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}}
}

func TestProfilePinsIdentityAndKeepsCredentialsPrivate(t *testing.T) {
	p := testProfile()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := mcpconfig.AtomicJSON(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Workspace != p.Workspace {
		t.Fatal(loaded, err)
	}
	if strings.Contains(fmt.Sprintf("%v %#v", loaded, loaded), "secret") {
		t.Fatal("diagnostic credential leak")
	}
	for _, change := range []func(*Profile){
		func(p *Profile) { p.URL = "http://cloud.example/mcp" },
		func(p *Profile) { p.URL = "https://user:secret@cloud.example/mcp" },
		func(p *Profile) { p.URL += "?token=secret" },
		func(p *Profile) { p.Scopes = []string{"admin"} },
		func(p *Profile) { p.Workspace = "" },
		func(p *Profile) { p.TokenURL = "http://auth.example/token" },
	} {
		copy := p
		change(&copy)
		if copy.Validate() == nil {
			t.Fatal("accepted invalid profile")
		}
	}
	unlock, err := mcpconfig.Lock(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := mcpconfig.Lock(path + ".lock"); err == nil {
		other()
		t.Fatal("concurrent refresh lock succeeded")
	}
	unlock()
	p.Token.AccessToken = "rotated"
	if err := mcpconfig.AtomicJSON(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.Token.AccessToken != "rotated" {
		t.Fatal(err)
	}
	if _, err = json.Marshal(loaded); err != nil {
		t.Fatal(err)
	}
}
