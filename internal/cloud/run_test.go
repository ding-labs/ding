package cloud

import (
	"path/filepath"
	"testing"
)

func TestCloudListenerCannotAccidentallyExposePlainHTTP(t *testing.T) {
	c := Config{PublicURL: "https://ding.example", DataDir: t.TempDir(), KeyFile: filepath.Join(t.TempDir(), "key")}
	if err := c.Validate(); err != nil || c.Listen != "127.0.0.1:8787" {
		t.Fatal(err)
	}
	c.Listen = "0.0.0.0:8787"
	if err := c.Validate(); err == nil {
		t.Fatal("public plaintext listener accepted")
	}
	c.TLSCert = "cert"
	if err := c.Validate(); err == nil {
		t.Fatal("incomplete TLS accepted")
	}
	c.TLSKey = "key"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.PublicURL = "https://user:secret@ding.example"
	if err := c.Validate(); err == nil {
		t.Fatal("credential-bearing public origin accepted")
	}
}
