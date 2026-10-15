package cloud

import (
	"context"
	"errors"
	"fmt"
	"github.com/ding-labs/ding/internal/identity"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/authkit"
	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/cloudbackup"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/hostingpolicy"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/webui"
)

type Config struct {
	Database            database.Config        `json:"database,omitempty"`
	Capacity            hostingpolicy.Capacity `json:"capacity,omitempty"`
	Push                PushConfig             `json:"push,omitempty"`
	AuthKitAPIKey       string                 `json:"authKitApiKey,omitempty"`
	Deployment          string                 `json:"deployment,omitempty"`
	LoginMethods        []string               `json:"loginMethods,omitempty"`
	Hosting             string                 `json:"hosting,omitempty"`
	Name                string                 `json:"name,omitempty"`
	ApprovedCommands    []string               `json:"approvedCommands,omitempty"`
	EnrollmentLimit     *int                   `json:"enrollmentLimit,omitempty"`
	OperatorListen      string                 `json:"operatorListen,omitempty"`
	PublicURL           string                 `json:"publicURL"`
	Listen              string                 `json:"listen"`
	DataDir             string                 `json:"dataDir"`
	KeyFile             string                 `json:"keyFile"`
	TLSCert             string                 `json:"tlsCert,omitempty"`
	TLSKey              string                 `json:"tlsKey,omitempty"`
	Identity            identity.Config        `json:"identity"`
	MCP                 *mcpconfig.HTTP        `json:"mcp,omitempty"`
	MCPBridgeClients    []string               `json:"mcpBridgeClients,omitempty"`
	AuthKitClients      []string               `json:"authKitClients,omitempty"`
	OpenAIAppsChallenge string                 `json:"openAIAppsChallenge,omitempty"`
}

func (c *Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if c.AuthKitAPIKey != "" && len(c.AuthKitClients) == 0 {
		return fmt.Errorf("AuthKit API key requires configured AuthKit clients")
	}
	if err := validateOpenAIChallenge(c.OpenAIAppsChallenge); err != nil {
		return err
	}
	if err := c.Capacity.Validate(); err != nil {
		return err
	}
	if err := c.Push.Validate(c.Capacity.Effective().MaxBodyBytes); err != nil {
		return err
	}
	if c.Hosting == "" {
		c.Hosting = "cloud"
	}
	if c.Hosting != "cloud" && c.Hosting != "self-hosted" {
		return fmt.Errorf("hosting must be cloud or self-hosted")
	}
	if c.LoginMethods == nil {
		c.LoginMethods = []string{"oidc"}
		if c.Hosting == "self-hosted" {
			c.LoginMethods = []string{"password"}
			if c.Identity.Issuer != "" {
				c.LoginMethods = append(c.LoginMethods, "oidc")
			}
		}
	}
	if len(c.LoginMethods) == 0 {
		return fmt.Errorf("at least one login method is required")
	}
	seen := map[string]bool{}
	for _, method := range c.LoginMethods {
		if (method != "password" && method != "oidc") || seen[method] {
			return fmt.Errorf("loginMethods must contain unique password or oidc entries")
		}
		seen[method] = true
	}
	if !seen["oidc"] && (c.Identity.Issuer != "" || c.Identity.ClientID != "" || c.Identity.ClientSecret != "" || len(c.Identity.Scopes) > 0) {
		return fmt.Errorf("identity configuration requires the oidc login method")
	}
	if len(c.MCPBridgeClients) > 32 || (len(c.MCPBridgeClients) > 0 && c.MCP == nil) {
		return fmt.Errorf("MCP bridge clients require configured public MCP and at most 32 native client IDs")
	}
	for _, client := range c.MCPBridgeClients {
		if client == "" || len(client) > 512 {
			return fmt.Errorf("invalid native MCP bridge client ID")
		}
	}
	if len(c.AuthKitClients) > 0 {
		if c.Hosting != "cloud" || c.MCP == nil || len(c.AuthKitClients) > 10 {
			return fmt.Errorf("AuthKit requires cloud MCP and at most ten registered client IDs")
		}
		for i, client := range c.AuthKitClients {
			if client == "" || len(client) > 512 || strings.ContainsAny(client, " \t\r\n") || client == c.Identity.ClientID || slices.Contains(c.AuthKitClients[:i], client) || slices.Contains(c.MCPBridgeClients, client) {
				return fmt.Errorf("invalid AuthKit client")
			}
		}
	}
	if c.MCP != nil && !seen["oidc"] {
		return fmt.Errorf("public MCP requires oidc login and an OAuth issuer")
	}
	if seen["oidc"] {
		if err := c.Identity.Validate(); err != nil {
			return err
		}
	}
	if c.Deployment != "" && c.Deployment != "railway" {
		return fmt.Errorf("unknown deployment mode")
	}
	if c.Deployment == "railway" && (c.Hosting != "cloud" || c.TLSCert != "" || c.TLSKey != "") {
		return fmt.Errorf("railway mode requires cloud hosting with TLS terminated at its edge")
	}
	if len(c.Name) > 100 {
		return fmt.Errorf("instance name exceeds 100 bytes")
	}
	if c.Hosting == "cloud" && len(c.ApprovedCommands) > 0 {
		return fmt.Errorf("cloud cannot approve host commands")
	}
	for _, revision := range c.ApprovedCommands {
		if !accountID.MatchString(revision) {
			return fmt.Errorf("approvedCommands must contain exact watch revisions")
		}
	}
	if c.EnrollmentLimit == nil {
		limit := MaxAccounts
		c.EnrollmentLimit = &limit
	}
	if *c.EnrollmentLimit < 0 || *c.EnrollmentLimit > MaxAccounts {
		return fmt.Errorf("enrollmentLimit must be 0–%d; zero closes new enrollment", MaxAccounts)
	}
	origin, err := mcpconfig.Endpoint(c.PublicURL, true)
	if err != nil {
		return fmt.Errorf("publicURL must be an HTTPS origin")
	}
	c.PublicURL = origin
	if c.MCP != nil {
		c.MCP.PublicURL = origin
		if err := c.MCP.ValidateAuthority(); err != nil {
			return fmt.Errorf("invalid MCP OAuth authority")
		}
		if c.MCP.Issuer != c.Identity.Issuer || c.MCP.Audience != origin+"/mcp" || len(c.MCP.Subjects) > 0 {
			return fmt.Errorf("MCP must share the browser issuer, use the exact public /mcp audience, and use workspace consent")
		}
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil || net.ParseIP(host) == nil {
		return fmt.Errorf("listen must be an explicit IP:port")
	}
	if c.OperatorListen != "" {
		host, _, err := net.SplitHostPort(c.OperatorListen)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("operatorListen must be loopback")
		}
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("configure both TLS certificate and key")
	}
	if !net.ParseIP(host).IsLoopback() && c.TLSCert == "" && c.Deployment != "railway" {
		return fmt.Errorf("non-loopback cloud listeners require TLS; use a loopback listener behind an HTTPS proxy")
	}
	if !filepath.IsAbs(c.DataDir) || !filepath.IsAbs(c.KeyFile) {
		return fmt.Errorf("dataDir and keyFile must be absolute paths")
	}
	return nil
}

func ValidateServerDirectory(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, "ding.db")); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("personal watch state cannot be used as server metadata; choose a fresh server data directory")
	}
	return nil
}

func Run(ctx context.Context, c Config, version string, out io.Writer) error {
	if err := ValidateServerDirectory(c.DataDir); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(c.DataDir, cloudbackup.Quarantine)); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("restored state is quarantined; review runner ownership and release it with the offline operator command")
	}
	assets := webui.Handler()
	if assets == nil {
		return fmt.Errorf("build ding-cloud with the console,mcpui tags and bundled assets")
	}
	key, err := loadKey(&c)
	if err != nil {
		return err
	}
	var provider identity.Provider
	if c.Identity.Issuer != "" {
		provider, err = identity.NewOIDC(ctx, c.PublicURL, c.Identity)
		if err != nil {
			return err
		}
	}
	storage, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		return err
	}
	defer storage.Close()
	ctx = storage.Context()
	if err = storage.CheckQuarantine(ctx); err != nil {
		return err
	}
	db := storage.Control
	if c.Deployment == "railway" || c.Database.Effective().Backend == "postgres" {
		if err := db.ConfigureDataKey(ctx, key); err != nil {
			return err
		}
	}
	if err := db.ConfigureHosting(ctx, c.Hosting); err != nil {
		return err
	}
	if err := db.ConfigureLoginMethods(ctx, c.LoginMethods); err != nil {
		return err
	}
	if slices.Contains(c.LoginMethods, "oidc") {
		if err := db.ConfigureOIDCIssuer(ctx, c.Identity.Issuer); err != nil {
			return err
		}
	}
	if slices.Contains(c.LoginMethods, "password") {
		workspaces, err := db.Workspaces(ctx)
		if err != nil || len(workspaces) == 0 {
			return fmt.Errorf("initialize built-in accounts with the host init command first")
		}
	}
	vault, err := state.NewVault(db, key)
	if err != nil {
		return err
	}
	client, closeHTTP := (egress.Guard{}).Client()
	defer closeHTTP()
	if c.Hosting == "self-hosted" {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	pool := NewPool(ctx, c.DataDir, db, vault, client)
	storage.ConfigurePool(pool)
	pool.Capacity = c.Capacity
	pool.Shared = c.Hosting == "self-hosted"
	pool.ApprovedCommands = append([]string(nil), c.ApprovedCommands...)
	defer pool.Close()
	stopMaintenance := pool.Maintain(ctx, out)
	defer stopMaintenance()
	if err := pool.FinishPendingDeletes(ctx); err != nil {
		return err
	}
	if err := pool.Start(ctx); err != nil {
		return err
	}
	app := &Server{Push: c.Push, OpenAIAppsChallenge: c.OpenAIAppsChallenge, AuthKitClients: slices.Clone(c.AuthKitClients), MCPBridgeClients: c.MCPBridgeClients, Railway: c.Deployment == "railway", LoginMethods: c.LoginMethods, Shared: pool.Shared, Name: c.Name, DB: db, Vault: vault, Pool: pool, PublicURL: c.PublicURL, Version: version, Assets: assets, Login: LoginFlow{DB: db, Vault: vault, Provider: provider, EnrollmentLimit: c.EnrollmentLimit, Shared: pool.Shared}}
	app.AuthKitIssuer = c.Identity.Issuer
	if c.AuthKitAPIKey != "" {
		app.ConsentRevoker = authkit.NewRevoker(c.AuthKitAPIKey)
	}
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
	if c.OperatorListen != "" {
		listener, err := operatorListener(c.OperatorListen)
		if err != nil {
			return err
		}
		metrics := &http.Server{Handler: http.HandlerFunc(pool.Metrics), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		defer metrics.Close()
		go func() { _ = metrics.Serve(listener) }()
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
	fmt.Fprintf(out, "Ding %s listening on %s; public origin %s.\n", c.Hosting, listener.Addr(), c.PublicURL)
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
