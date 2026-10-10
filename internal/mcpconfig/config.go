// Package mcpconfig reads private operator configuration. Credentials never enter
// tool arguments, results, or diagnostics.
package mcpconfig

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var ErrPrivate = errors.New("configuration must be a private regular file owned by this user")
var ErrConfig = errors.New("invalid private configuration; check endpoint, permissions, and required fields")
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Connection struct {
	DaemonURL string `json:"daemon_url"`
	Token     string `json:"token"`
	GrantID   string `json:"grant_id"`
}

func (c Connection) String() string   { return "Ding connection (credential redacted)" }
func (c Connection) GoString() string { return c.String() }

type HTTP struct {
	PublicURL string            `json:"public_url"`
	Issuer    string            `json:"issuer"`
	JWKSURI   string            `json:"jwks_uri"`
	Audience  string            `json:"audience"`
	Subjects  map[string]string `json:"subjects"`
	Algorithm string            `json:"algorithm"`
}

func Endpoint(value string, httpsOnly bool) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", ErrConfig
	}
	loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && (httpsOnly || u.Scheme != "http" || !loopback) {
		return "", ErrConfig
	}
	return strings.TrimSuffix(value, "/"), nil
}

func (c *Connection) Validate() error {
	u, err := Endpoint(c.DaemonURL, false)
	if err != nil || !hex64.MatchString(c.GrantID) || !strings.HasPrefix(c.Token, "ding_mcp_") || !hex64.MatchString(strings.TrimPrefix(c.Token, "ding_mcp_")) {
		return ErrConfig
	}
	c.DaemonURL = u
	return nil
}

func (h *HTTP) Validate() error {
	u, err := Endpoint(h.PublicURL, true)
	if err != nil {
		return ErrConfig
	}
	h.PublicURL = u
	for _, value := range []string{h.Issuer, h.JWKSURI} {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return ErrConfig
		}
	}
	if h.Audience == "" || len(h.Subjects) == 0 {
		return ErrConfig
	}
	if h.Algorithm == "" {
		h.Algorithm = "RS256"
	}
	if h.Algorithm == "Ed25519" {
		h.Algorithm = "EdDSA"
	} // Backward-compatible config alias; JWT uses EdDSA.
	switch h.Algorithm {
	case "RS256", "RS384", "RS512", "ES256", "ES384", "EdDSA":
	default:
		return ErrConfig
	}
	for subject, path := range h.Subjects {
		if subject == "" || !filepath.IsAbs(path) {
			return ErrConfig
		}
	}
	return nil
}

func LoadConnection(path string) (Connection, error) {
	var c Connection
	if err := ReadPrivateJSON(path, &c, true); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func LoadHTTP(path string) (HTTP, error) {
	var h HTTP
	if err := ReadPrivateJSON(path, &h, true); err != nil {
		return h, err
	}
	if err := h.Validate(); err != nil {
		return h, err
	}
	for _, path := range h.Subjects {
		if _, err := LoadConnection(path); err != nil {
			return h, ErrConfig
		}
	}
	return h, nil
}

func ReadPrivateJSON(path string, dest any, strict bool) error {
	f, err := openPrivate(path, false)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return ErrPrivate
	}
	d := json.NewDecoder(io.LimitReader(f, (1<<20)+1))
	if strict {
		d.DisallowUnknownFields()
	}
	if d.Decode(dest) != nil {
		return ErrConfig
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return ErrConfig
	}
	return nil
}

// CreatePrivate reserves a new file before granting access. It never overwrites
// a pairing. Platform helpers enforce permissions on the opened handle.
func CreatePrivate(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return openPrivate(path, true)
}

// CheckPrivate validates ownership and permissions without interpreting content.
func CheckPrivate(path string) error {
	f, err := openPrivate(path, false)
	if err != nil {
		return err
	}
	return f.Close()
}

// OpenPrivate opens an existing, owner-only regular file without following links.
func OpenPrivate(path string) (*os.File, error) { return openPrivate(path, false) }

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return configPath(runtime.GOOS, home, os.Getenv("XDG_CONFIG_HOME"), os.Getenv("LOCALAPPDATA"))
}

func configPath(goos, home, xdg, local string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Ding", "mcp.json")
	case "windows":
		return filepath.Join(local, "Ding", "mcp.json")
	default:
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, "Ding", "mcp.json")
	}
}

func DefaultState() string { dir, _ := os.UserConfigDir(); return filepath.Join(dir, "ding", "watch") }
