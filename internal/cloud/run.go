package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/webui"
)

type Config struct {
	PublicURL string          `json:"publicURL"`
	Listen    string          `json:"listen"`
	DataDir   string          `json:"dataDir"`
	KeyFile   string          `json:"keyFile"`
	TLSCert   string          `json:"tlsCert,omitempty"`
	TLSKey    string          `json:"tlsKey,omitempty"`
	Identity  IdentityConfig  `json:"identity"`
	MCP       *mcpconfig.HTTP `json:"mcp,omitempty"`
}

func (c *Config) Validate() error {
	origin, err := mcpconfig.Endpoint(c.PublicURL, true)
	if err != nil {
		return fmt.Errorf("publicURL must be an HTTPS origin")
	}
	c.PublicURL = origin
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil || net.ParseIP(host) == nil {
		return fmt.Errorf("listen must be an explicit IP:port")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("configure both TLS certificate and key")
	}
	if !net.ParseIP(host).IsLoopback() && c.TLSCert == "" {
		return fmt.Errorf("non-loopback cloud listeners require TLS; use a loopback listener behind an HTTPS proxy")
	}
	if !filepath.IsAbs(c.DataDir) || !filepath.IsAbs(c.KeyFile) {
		return fmt.Errorf("dataDir and keyFile must be absolute paths")
	}
	return nil
}

func Run(ctx context.Context, c Config, version string, out io.Writer) error {
	if err := c.Validate(); err != nil {
		return err
	}
	assets := webui.Handler()
	if assets == nil {
		return fmt.Errorf("build ding-cloud with the console,mcpui tags and bundled assets")
	}
	f, err := mcpconfig.OpenPrivate(c.KeyFile)
	if err != nil {
		return fmt.Errorf("open external encryption key: %w", err)
	}
	key, readErr := io.ReadAll(io.LimitReader(f, 33))
	closed := f.Close()
	if readErr != nil || closed != nil || len(key) != 32 {
		return fmt.Errorf("keyFile must contain exactly 32 private bytes")
	}
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return err
	}
	c.DataDir, err = filepath.EvalSymlinks(c.DataDir)
	if err != nil {
		return err
	}
	keyPath, err := filepath.EvalSymlinks(c.KeyFile)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(c.DataDir, keyPath)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("keep the encryption key outside the data and backup volume")
	}
	provider, err := NewIdentity(ctx, c.PublicURL, c.Identity)
	if err != nil {
		return err
	}
	db, err := state.Open(ctx, c.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	vault, err := state.NewVault(db, key)
	if err != nil {
		return err
	}
	client, closeHTTP := (egress.Guard{}).Client()
	defer closeHTTP()
	pool := NewPool(ctx, c.DataDir, db, vault, client)
	defer pool.Close()
	if err := pool.Start(ctx); err != nil {
		return err
	}
	app := &Server{DB: db, Vault: vault, Pool: pool, PublicURL: c.PublicURL, Version: version, Assets: assets, Login: LoginFlow{DB: db, Vault: vault, Provider: provider}}
	if c.MCP != nil {
		if c.MCP.Issuer != c.Identity.Issuer {
			return fmt.Errorf("MCP and browser identity must use the same issuer and stable subject")
		}
		var closeMCP func()
		app.MCP, closeMCP, err = app.MCPHandler(*c.MCP, nil)
		if err != nil {
			return err
		}
		defer closeMCP()
	}
	handler, err := app.Handler()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() {
		if c.TLSCert != "" {
			done <- server.ServeTLS(listener, c.TLSCert, c.TLSKey)
		} else {
			done <- server.Serve(listener)
		}
	}()
	fmt.Fprintf(out, "Ding Cloud listening on %s; public origin %s; cohort cap %d.\n", listener.Addr(), c.PublicURL, MaxAccounts)
	select {
	case err = <-done:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if stopErr := server.Shutdown(shutdown); stopErr != nil {
		_ = server.Close()
		err = errors.Join(err, stopErr)
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, pool.Close())
}
