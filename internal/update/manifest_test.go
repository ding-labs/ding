package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReleaseTrustAndCompatibility(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(pub)
	now := time.Now().UTC()
	m := Manifest{Protocol: 1, Version: "v1.2.3", Channel: "stable", Schema: 4, PublishedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), Artifacts: []Artifact{{OS: "linux", Arch: "amd64", URL: "https://github.com/ding-labs/ding/releases/download/v1.2.3/ding_linux_amd64.tar.gz", SHA256: strings.Repeat("a", 64), Bytes: 123}}}
	verify := func(m Manifest, channel string, key string) error {
		b, _ := json.Marshal(m)
		sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, b)))
		_, err := Verify(b, sig, key, channel, now)
		return err
	}
	if err := verify(m, "stable", key); err != nil {
		t.Fatal(err)
	}
	if err := verify(m, "preview", key); err == nil {
		t.Fatal("accepted channel mismatch")
	}
	if err := verify(m, "stable", ""); err == nil {
		t.Fatal("accepted missing trust anchor")
	}
	if _, err := m.Select("linux", "amd64", "1.2.2", 4); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version string
		schema  int
	}{{"1.2.4", 4}, {"1.2.3", 4}, {"dev", 4}, {"1.2.2", 5}} {
		if _, err := m.Select("linux", "amd64", tc.version, tc.schema); err == nil {
			t.Fatal("unsafe selection", tc)
		}
	}
	for _, alter := range []func(*Manifest){func(m *Manifest) { m.ExpiresAt = now.Add(-time.Second) }, func(m *Manifest) { m.Artifacts[0].URL = "https://evil.example/ding" }, func(m *Manifest) { m.Artifacts[0].Bytes = MaxArtifactBytes + 1 }, func(m *Manifest) { m.Protocol = 2 }} {
		b, _ := json.Marshal(m)
		var bad Manifest
		_ = json.Unmarshal(b, &bad)
		alter(&bad)
		if err := verify(bad, "stable", key); err == nil {
			t.Fatal("accepted unsafe metadata")
		}
	}
	b, _ := json.Marshal(m)
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, b)))
	b[0] = ' '
	if _, err := Verify(b, sig, key, "stable", now); err == nil {
		t.Fatal("accepted tampered metadata")
	}
}
