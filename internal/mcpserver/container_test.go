package mcpserver_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Opt-in artifact qualification. Both JWKS and the synthetic daemon use real
// HTTPS with a test CA. No public issuer, user instance, or credentials are used.
func TestContainerTLSAndAuthentication(t *testing.T) {
	image := os.Getenv("DING_MCP_CONTAINER")
	if image == "" || runtime.GOOS == "windows" {
		t.Skip("set DING_MCP_CONTAINER to qualify a built image on Unix")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Ding test CA"}, DNSNames: []string{"host.docker.internal"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, _ := jwk.Import(key)
	_ = private.Set(jwk.KeyIDKey, "container-test")
	public, _ := jwk.PublicKeyOf(private)
	keys := jwk.NewSet()
	_ = keys.AddKey(public)
	var daemonCalls atomic.Int32
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwks" {
			_ = json.NewEncoder(w).Encode(keys)
			return
		}
		if r.URL.Path != "/v1/integrations/capabilities" || r.Header.Get("Authorization") != "Bearer ding_mcp_"+strings.Repeat("a", 64) {
			http.Error(w, "denied", 403)
			return
		}
		daemonCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "ding.ing/v1alpha1", "data": map[string]any{"version": "ding.integration/v1", "instance": "container-qualified", "grant": map[string]any{}, "eventsSubscriptions": false}})
	}))
	upstream.Listener.Close()
	upstream.Listener, err = net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	upstream.StartTLS()
	defer upstream.Close()
	issuer := fmt.Sprintf("https://host.docker.internal:%d", upstream.Listener.Addr().(*net.TCPAddr).Port)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		t.Helper()
		f, err := mcpconfig.CreatePrivate(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(f).Encode(value); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	write("alice.json", mcpconfig.Connection{DaemonURL: issuer, Token: "ding_mcp_" + strings.Repeat("a", 64), GrantID: strings.Repeat("b", 64)})
	write("http.json", mcpconfig.HTTP{PublicURL: "https://ding.example", Issuer: issuer, JWKSURI: issuer + "/jwks", Audience: "ding", Subjects: map[string]string{"alice": "/config/alice.json"}, Algorithm: "RS256"})
	docker := func(args ...string) string {
		t.Helper()
		b, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("container command: %v %s", err, b)
		}
		return strings.TrimSpace(string(b))
	}
	args := []string{"run", "-d", "--rm", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src=" + dir + ",dst=/config,readonly", "--env", "SSL_CERT_FILE=/config/ca.pem", "-p", "127.0.0.1::7677"}
	if runtime.GOOS == "linux" {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}
	args = append(args, image, "serve", "--transport", "http", "--http-config", "/config/http.json", "--host", "0.0.0.0")
	cid := docker(args...)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", cid).Run()
	}()
	address := "http://" + docker("port", cid, "7677/tcp")
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	for {
		r, _ := http.NewRequestWithContext(ctx, "GET", address+"/.well-known/oauth-protected-resource/mcp", nil)
		r.Host = "ding.example"
		response, err := client.Do(r)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("container never became ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
	token := jwt.New()
	for k, v := range map[string]any{"iss": issuer, "aud": "ding", "sub": "alice", "scope": "ding:inspect", "exp": time.Now().Add(time.Hour)} {
		_ = token.Set(k, v)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), private))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		bearer string
		status int
	}{{"", 401}, {"invalid", 401}, {string(signed), 200}} {
		body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ding_get_capabilities","arguments":{}}}`)
		r, _ := http.NewRequestWithContext(ctx, "POST", address+"/mcp", bytes.NewReader(body))
		r.Host = "ding.example"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-11-25")
		if tc.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != tc.status {
			t.Fatalf("authentication status %d, expected %d", response.StatusCode, tc.status)
		}
		if tc.status == 200 && !bytes.Contains(data, []byte("container-qualified")) {
			t.Fatalf("container TLS tool call failed: %s", data)
		}
	}
	if daemonCalls.Load() != 1 {
		t.Fatal("unauthorized request reached upstream", daemonCalls.Load())
	}
}
