package cloud

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/ding-labs/ding/internal/pgtest"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/watchrun"
	"golang.org/x/time/rate"
)

type scaleLab struct {
	internalObservations int64
	storage              *ServerStorage
	databaseConfig       database.Config
	fixtureDatabase      string
	server               *Server
	http                 *httptest.Server
	tenants              []*Tenant
	producers            []PushProducer
	root                 string
	receiver             *scaleReceiver
	key                  []byte
	control              http.Handler
	controlMu            sync.RWMutex
	preloadFills         []PushBenchmarkResult
}

func newScaleLab(t *testing.T, c scaleConfig) *scaleLab {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	dbConfig := database.Config{}
	fixtureName := ""
	if c.Backend == "postgres" {
		f := pgtest.New(t)
		f.SetAppEnvironment(t, "DING_SCALE_DATABASE_APP")
		f.SetMigrationEnvironment(t, "DING_SCALE_DATABASE_MIGRATION")
		dbConfig = database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_SCALE_DATABASE_APP"}, MaxOpenConns: c.PoolLimit, QueryMode: c.QueryMode}
		if err := MigrateDatabase(ctx, Config{DataDir: root, Database: dbConfig}, "DING_SCALE_DATABASE_MIGRATION"); err != nil {
			t.Fatal(err)
		}
		fixtureName = f.Name
		f.App.Close()
		f.Migration.Close()
	}
	storage, err := openScaleStorage(ctx, root, dbConfig, c.CredentialReaders)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	ctx = storage.Context()
	db := storage.Control
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	vault, err := state.NewVault(db, key)
	if err != nil {
		t.Fatal(err)
	}
	client, closeHTTP := (egress.Guard{}).Client()
	t.Cleanup(closeHTTP)
	pool := NewPool(ctx, root, db, vault, client)
	storage.ConfigurePool(pool)
	t.Cleanup(func() { pool.Close() })

	// Lab-only expansion bypasses the production 128 MiB backup envelope.
	// The real store, WAL/FULL durability, auth, and evaluator are retained.
	if !c.Defaults {
		pool.Capacity.MaxEntities = 10000
	}
	s := &Server{DB: db, Vault: vault, Pool: pool, PublicURL: "https://ding.example", Version: "local-scale"}
	if !c.Defaults {
		s.Push = PushConfig{HostConcurrent: 256, WorkspaceConcurrent: 256, WorkspaceRate: 10000, WorkspaceBurst: 10000, ProducerRate: 10000, ProducerBurst: 10000, BytesPerSecond: 1 << 30, ByteBurst: 1 << 30, MaxReceipts: 1000000}
	}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Defaults {
		s.pushGlobal = rate.NewLimiter(100000, 100000)
	}
	lab := &scaleLab{server: s, root: root, key: key, storage: storage, databaseConfig: dbConfig, fixtureDatabase: fixtureName}
	t.Cleanup(func() {
		out := os.Getenv("DING_SCALE_OUTPUT")
		if !t.Failed() || out == "" {
			return
		}
		lab.server.Pool.Close()
		lab.storage.Close()
		destination := filepath.Join(out, "failed-state")
		if os.Getenv("DING_SCALE_EXTERNAL") == "container" {
			// Move outside testing's temporary parent but stay on the same
			// volume so the launcher can archive it after the process exits.
			parent := filepath.Dir(root)
			destination = filepath.Join(filepath.Dir(parent), "failed-"+filepath.Base(parent))
		}
		if _, err := os.Stat(destination); !os.IsNotExist(err) {
			t.Errorf("failed-state destination already exists or is inaccessible")
			return
		}
		if err := os.Rename(root, destination); err != nil {
			t.Errorf("preserving failed fixture state: %v", err)
		}
	})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/lab/") {
			lab.controlMu.RLock()
			control := lab.control
			lab.controlMu.RUnlock()
			if control == nil {
				w.WriteHeader(404)
			} else {
				control.ServeHTTP(w, r)
			}
			return
		}
		if c.Noop {
			if _, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 1<<20)); err != nil {
				w.WriteHeader(413)
				return
			}
			w.WriteHeader(202)
			fmt.Fprintf(w, `{"data":{"acceptanceId":%q}}`, r.Header.Get("Idempotency-Key"))
			return
		}
		r.Host = "ding.example"
		h.ServeHTTP(w, r)
	}))
	privateHost := os.Getenv("DING_SCALE_PRIVATE_HOST")
	if privateHost != "" {
		if !strings.HasSuffix(privateHost, ".railway.internal") || os.Getenv("DING_TEST_RAILWAY_PROJECT") == "" || os.Getenv("DING_TEST_RAILWAY_PROJECT") != os.Getenv("RAILWAY_PROJECT_ID") {
			t.Fatal("private scale fixture requires matching Railway project authorization")
		}
		certificate, err := tls.LoadX509KeyPair(os.Getenv("DING_SCALE_TLS_CERT"), os.Getenv("DING_SCALE_TLS_KEY"))
		if err != nil {
			t.Fatal("load scale fixture TLS certificate")
		}
		server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	}
	if os.Getenv("DING_SCALE_EXTERNAL") == "container" || privateHost != "" {
		server.Listener.Close()
		server.Listener, err = net.Listen("tcp", ":8443")
		if err != nil {
			t.Fatal(err)
		}
	}
	server.StartTLS()
	if privateHost != "" {
		server.URL = "https://" + privateHost + ":8443"
	}
	if os.Getenv("DING_SCALE_EXTERNAL") == "container" {
		server.URL = "https://127.0.0.1:8443"
	}
	t.Cleanup(server.Close)
	lab.http = server
	if c.Receiver != "" {
		lab.receiver = newScaleReceiver(t, c.Receiver, c.Seconds, filepath.Join(root, "receiver.jsonl"))
		pool.http.Transport = scaleDeliveryTransport{receiver: lab.receiver, next: pool.http.Transport}
	}
	bodies, err := c.bodies()
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.Workspaces {
		// Lab-only capacity expansion; the production account policy is unchanged.
		enrolled, err := db.Enroll(ctx, "local-scale", fmt.Sprintf("user-%d", i), max(MaxAccounts, c.Workspaces))
		if err != nil {
			t.Fatal(err)
		}
		if lab.receiver != nil {
			if err := vault.Put(ctx, enrolled.Workspace.ID, "SCALE_WEBHOOK", lab.receiver.endpoint); err != nil {
				t.Fatal(err)
			}
		}
		tenant, err := scaleTenant(pool, enrolled.Workspace, c.Defaults, c.DeliveryWorkers)
		if err != nil {
			t.Fatal(err)
		}
		for _, watchID := range c.watchIDs() {
			manifest := strings.Replace(c.manifest(), "metadata: {id: ci}", "metadata: {id: "+watchID+"}", 1)
			if _, err := tenant.App.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
				t.Fatal(err)
			}
		}
		principal, err := db.Principal(ctx, enrolled.User, enrolled.Workspace.ID, "browser", nil)
		if err != nil {
			t.Fatal(err)
		}
		life, err := s.producerLifetimes(ctx, enrolled.Workspace.ID, c.watchIDs())
		if err != nil {
			t.Fatal(err)
		}
		grant, err := db.CreateBoundService(ctx, principal, "scale", []string{"ingest", "inspect"}, c.watchIDs(), 30, life)
		if err != nil {
			t.Fatal(err)
		}
		env := fmt.Sprintf("DING_LOCAL_SCALE_%d", i)
		t.Setenv(env, grant.Token)
		for _, watchID := range c.watchIDs() {
			for index, body := range bodies {
				producer := PushProducer{URL: server.URL + "/v1/ingest/" + watchID, TokenEnv: env, Body: body}
				if c.PayloadMix {
					producer.Weight = []int{16, 3, 1}[index]
					producer.Label = []string{"webhook10k", "batch18_100k", "batch100_600k"}[index]
				}
				lab.producers = append(lab.producers, producer)
			}
		}
		lab.tenants = append(lab.tenants, tenant)
	}
	if len(lab.producers) > 10000 {
		t.Fatal("too many fixture variants")
	}
	return lab
}

// Configure limits before workers start; changing App.Limits on a running
// production Pool tenant would race with retention and acquisition workers.
func scaleTenant(pool *Pool, w state.Workspace, defaults bool, deliveryWorkers ...int) (*Tenant, error) {
	tenant, err := prepareScaleTenant(pool, w, defaults, deliveryWorkers...)
	if err == nil {
		startScaleTenant(tenant)
	}
	return tenant, err
}

func prepareScaleTenant(pool *Pool, w state.Workspace, defaults bool, deliveryWorkers ...int) (*Tenant, error) {
	db, err := pool.workspaceStorage().Open(pool.ctx, w.ID)
	if err != nil {
		return nil, err
	}
	app := watchrun.New(db)
	app.Integration = integrationpolicy.Hosted(w.ID, w.Name, false)
	app.Limits = pool.Capacity.Limits()
	if !defaults {
		app.Limits.MaxBytes = 4 << 30
		app.Limits.MaxPending = 100000
	}
	app.AcquisitionWorkers, app.DeliveryWorkers = 1, 1
	if len(deliveryWorkers) > 0 {
		app.DeliveryWorkers = deliveryWorkers[0]
	}
	app.PollInterval = time.Second
	app.SchedulingJitter = 5 * time.Second
	app.Lookup = pool.vault.Lookup(w.ID)
	app.HTTP.Lookup = app.Lookup
	app.HTTP.Client = pool.http
	app.Output = io.Discard
	app.ValidateBundle = pool.Capacity.Policy
	run, stop := context.WithCancel(pool.ctx)
	tenant := &Tenant{Workspace: w, App: app, stop: stop, done: make(chan struct{}), ctx: run}
	pool.mu.Lock()
	pool.tenants[w.ID] = tenant
	pool.mu.Unlock()
	return tenant, nil
}

func startScaleTenant(tenant *Tenant) {
	go func() {
		tenant.err = tenant.App.Run(tenant.ctx)
		tenant.failed.Store(true)
		close(tenant.done)
	}()
}

func TestScaleLabUsesIsolatedHostedPath(t *testing.T) {
	c := scaleConfig{Name: "smoke", Rate: 10, Seconds: 1, Workspaces: 2, Outputs: 18}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	if lab.tenants[0].Workspace.ID == lab.tenants[1].Workspace.ID {
		t.Fatal("shared workspace")
	}
	r, err := lab.http.Client().Post(lab.producers[0].URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("fixture disabled authorization", r.StatusCode)
	}
	if filepath.Dir(lab.root) == "/data" {
		t.Fatal("unexpected production path")
	}
}

func openScaleStorage(ctx context.Context, root string, c database.Config, readers int) (*ServerStorage, error) {
	if c.Effective().Backend == "postgres" {
		return OpenServerStorage(ctx, root, c)
	}
	db, err := state.OpenSQLite(ctx, root, readers)
	if err != nil {
		return nil, err
	}
	return &ServerStorage{Control: db, workspaces: sqliteWorkspaces{root}, ctx: ctx}, nil
}
