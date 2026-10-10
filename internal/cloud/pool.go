package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

const MaxAccounts = 100

var accountID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Tenant struct {
	Account state.Account
	App     *watchrun.App
	stop    context.CancelFunc
	done    chan struct{}
	err     error
	ctx     context.Context
	failed  atomic.Bool
}
type Pool struct {
	root     string
	db       *state.DB
	vault    *state.Vault
	http     *http.Client
	ctx      context.Context
	mu       sync.Mutex
	tenants  map[string]*Tenant
	closed   bool
	deleting map[string]bool
}

// NewPool requires the control DB's host lock to remain held until Close returns.
// The HTTP override exists for isolated benchmarks/tests; the service entrypoint
// must construct it with Guard.Client, never an unguarded default transport.
func NewPool(ctx context.Context, root string, db *state.DB, vault *state.Vault, client *http.Client) *Pool {
	shared := *client
	shared.Transport = egress.NewHostLimits(egress.NewLimit(client.Transport, 32))
	return &Pool{root: root, db: db, vault: vault, http: &shared, ctx: ctx, tenants: map[string]*Tenant{}, deleting: map[string]bool{}}
}

func (p *Pool) Get(ctx context.Context, id string) (*Tenant, error) {
	if !accountID.MatchString(id) {
		return nil, fmt.Errorf("invalid workspace")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.deleting[id] {
		return nil, fmt.Errorf("cloud worker is stopping")
	}
	if t, ok := p.tenants[id]; ok {
		return t, nil
	}
	if len(p.tenants) >= MaxAccounts {
		return nil, fmt.Errorf("cloud worker enrollment capacity reached")
	}
	account, err := p.db.Account(ctx, id)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(ctx, filepath.Join(p.root, "workspaces", id))
	if err != nil {
		return nil, err
	}
	app := watchrun.New(db)
	app.Limits = Limits()
	app.AcquisitionWorkers, app.DeliveryWorkers = 1, 1
	app.PollInterval = time.Second
	app.SchedulingJitter = 5 * time.Second
	app.Lookup = p.vault.Lookup(id)
	app.HTTP.Lookup = app.Lookup
	app.Output = io.Discard
	client := *p.http
	client.Transport = budgetTransport{p.http.Transport, p.db, id}
	app.HTTP.Client = &client
	app.ValidateBundle = Policy
	// Revalidate persisted definitions before any worker starts. Database uploads
	// are not a tenant API; a local/older store still cannot enable command I/O.
	records, err := app.List(ctx)
	if err == nil {
		if len(records) > Limits().MaxWatches {
			active := 0
			for _, r := range records {
				if r.Status != "deleted" {
					active++
				}
			}
			if active > Limits().MaxWatches {
				err = fmt.Errorf("workspace exceeds active watch limit")
			}
		}
		for _, r := range records {
			if err != nil {
				break
			}
			if r.Status != "deleted" {
				err = Policy(plan.Bundle{Watches: []plan.Compiled{r.Plan}})
				if err != nil {
					break
				}
			}
		}
	}
	if err == nil {
		err = db.View(ctx, func(tx *store.Tx) error {
			destinations, err := tx.Destinations()
			if err != nil {
				return err
			}
			bundle := plan.Bundle{}
			for _, d := range destinations {
				bundle.Destinations = append(bundle.Destinations, plan.CompiledDestination{Definition: d})
			}
			return Policy(bundle)
		})
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("workspace execution policy requires repair: %w", err)
	}
	run, stop := context.WithCancel(p.ctx)
	t := &Tenant{Account: account, App: app, stop: stop, done: make(chan struct{}), ctx: run}
	p.tenants[id] = t
	go func() { t.err = app.Run(run); t.failed.Store(true); close(t.done) }()
	return t, nil
}

func (p *Pool) Start(ctx context.Context) error {
	accounts, err := p.db.Accounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) > MaxAccounts {
		return fmt.Errorf("stored cohort exceeds this worker's configured capacity")
	}
	for _, a := range accounts {
		if _, err := p.Get(ctx, a.ID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	list := make([]*Tenant, 0, len(p.tenants))
	for _, t := range p.tenants {
		t.stop()
		list = append(list, t)
	}
	p.mu.Unlock()
	var errs []error
	for _, t := range list {
		<-t.done
		errs = append(errs, t.err, t.App.Store.Close())
	}
	return errors.Join(errs...)
}

func (p *Pool) Delete(ctx context.Context, id string) error {
	if !accountID.MatchString(id) {
		return fmt.Errorf("invalid workspace")
	}
	p.mu.Lock()
	p.deleting[id] = true
	t := p.tenants[id]
	if t != nil {
		t.stop()
	}
	p.mu.Unlock()
	if t != nil {
		select {
		case <-t.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := t.App.Store.Close(); err != nil {
			return err
		}
	}
	if t == nil {
		path := filepath.Join(p.root, "workspaces", id)
		if _, err := os.Stat(path); err == nil {
			owned, err := store.Open(ctx, path)
			if err != nil {
				return err
			}
			if err := owned.Close(); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(p.root, "workspaces", id)); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.tenants, id)
	delete(p.deleting, id)
	p.mu.Unlock()
	return nil
}
func (p *Pool) Ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	for id, t := range p.tenants {
		if !p.deleting[id] && t.failed.Load() {
			return false
		}
	}
	return true
}
