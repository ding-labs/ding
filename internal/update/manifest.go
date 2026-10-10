// Package update verifies release metadata before staging or executing updates.
package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const MaxArtifactBytes = 256 << 20

// PublicKey is set at release build time from a reviewed public signing key.
// Development builds fail closed; no key is fetched from update metadata.
var PublicKey string

type Manifest struct {
	Protocol    int        `json:"protocol"`
	Version     string     `json:"version"`
	Channel     string     `json:"channel"`
	Schema      int        `json:"schema"`
	PublishedAt time.Time  `json:"publishedAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	Artifacts   []Artifact `json:"artifacts"`
}
type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func Verify(data, signature []byte, key, channel string, now time.Time) (Manifest, error) {
	var m Manifest
	public, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return m, fmt.Errorf("this build has no valid trusted update key; use the official signed package")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || len(data) > 1<<20 || !ed25519.Verify(public, data, sig) {
		return m, fmt.Errorf("release signature verification failed")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, fmt.Errorf("invalid release metadata")
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return m, fmt.Errorf("trailing release metadata")
	}
	if m.Protocol != 1 || !semver.IsValid(m.Version) || semver.Canonical(m.Version) != m.Version || m.Schema < 1 {
		return m, fmt.Errorf("unsupported release metadata")
	}
	if m.Channel != channel || (channel != "stable" && channel != "preview") || (channel == "stable" && semver.Prerelease(m.Version) != "") {
		return m, fmt.Errorf("release channel mismatch")
	}
	if m.PublishedAt.After(now.Add(5*time.Minute)) || !now.Before(m.ExpiresAt) || !m.ExpiresAt.After(m.PublishedAt) || m.ExpiresAt.Sub(m.PublishedAt) > 90*24*time.Hour {
		return m, fmt.Errorf("release metadata expired or has invalid dates")
	}
	seen := map[string]bool{}
	for _, a := range m.Artifacts {
		platform := a.OS + "/" + a.Arch
		if seen[platform] {
			return m, fmt.Errorf("duplicate release platform")
		}
		seen[platform] = true
		u, err := url.Parse(a.URL)
		prefix := "/ding-labs/ding/releases/download/" + m.Version + "/"
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, prefix) || strings.Contains(strings.TrimPrefix(u.Path, prefix), "/") {
			return m, fmt.Errorf("artifact must be an official versioned HTTPS release asset")
		}
		hash, err := hex.DecodeString(a.SHA256)
		if err != nil || len(hash) != 32 || a.Bytes < 1 || a.Bytes > MaxArtifactBytes {
			return m, fmt.Errorf("invalid artifact size or digest")
		}
	}
	return m, nil
}

func (m Manifest) Select(goos, arch, current string, schema int) (Artifact, error) {
	version := "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(version) {
		return Artifact{}, fmt.Errorf("development builds require an explicit packaged release installation")
	}
	if semver.Compare(m.Version, version) <= 0 {
		return Artifact{}, fmt.Errorf("no newer release is available")
	}
	// Schema-changing upgrades require a separate migration-qualified installer.
	// An old binary must never be rolled back onto a migrated database.
	if m.Schema != schema {
		return Artifact{}, fmt.Errorf("database schema changes require the release migration procedure")
	}
	for _, a := range m.Artifacts {
		if a.OS == goos && a.Arch == arch {
			return a, nil
		}
	}
	return Artifact{}, fmt.Errorf("release has no artifact for %s/%s", goos, arch)
}
