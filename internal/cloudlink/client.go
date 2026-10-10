// Package cloudlink connects a local client to an optional HTTPS cloud service.
// It does not depend on cloud workers, account storage, or an identity SDK.
package cloudlink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

type Connection struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	Workspace string    `json:"workspace"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (c Connection) String() string   { return "Ding Cloud connection (credential redacted)" }
func (c Connection) GoString() string { return c.String() }
func (c Connection) Client() (control.Client, error) {
	endpoint, err := mcpconfig.Endpoint(c.URL, true)
	if err != nil {
		return control.Client{}, err
	}
	return control.Client{URL: endpoint, Token: c.Token, HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func Load(dir string) (Connection, error) {
	var c Connection
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "cloud.json"), &c, true)
	if err != nil {
		return c, err
	}
	if len(c.Token) != 64 || len(c.Workspace) != 64 {
		return c, fmt.Errorf("invalid cloud connection")
	}
	_, err = c.Client()
	return c, err
}
func Proof() (string, string) {
	var b [32]byte
	rand.Read(b[:])
	verifier := hex.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(verifier))
	return verifier, hex.EncodeToString(sum[:])
}
func Call[T any](ctx context.Context, c control.Client, method, path string, input any) (T, error) {
	var result T
	raw, err := c.Call(ctx, method, path, input)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}

type Device struct {
	ID   string `json:"id"`
	URL  string `json:"verificationURL"`
	Code string `json:"code"`
}
type Claim struct {
	Ready     bool      `json:"ready"`
	Token     string    `json:"token"`
	Workspace string    `json:"workspace"`
	ExpiresAt time.Time `json:"expiresAt"`
}
