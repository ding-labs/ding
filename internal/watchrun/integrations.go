package watchrun

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

const IntegrationVersion = "ding.integration/v1"

func IntegrationTokenHash(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}
func integrationID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

type GrantRequest struct {
	SecretRefs       []string `json:"secretRefs"`
	Name             string   `json:"name"`
	Scopes           []string `json:"scopes"`
	CommandRevisions []string `json:"commandRevisions"`
	Days             int      `json:"days"`
}
type GrantPairing struct {
	Grant store.IntegrationGrant `json:"grant"`
	Token string                 `json:"token"`
}

func (a *App) CreateIntegrationGrant(ctx context.Context, r GrantRequest) (GrantPairing, error) {
	var out GrantPairing
	if len(r.Name) < 1 || len(r.Name) > 100 || len(r.Scopes) == 0 || len(r.Scopes) > 4 || r.Days < 1 || r.Days > 365 || len(r.CommandRevisions) > 100 || len(r.SecretRefs) > 100 {
		return out, fmt.Errorf("invalid integration grant")
	}
	for _, scope := range r.Scopes {
		if !slices.Contains([]string{"inspect", "preview", "manage", "retry"}, scope) {
			return out, fmt.Errorf("unknown integration scope")
		}
	}
	for _, revision := range r.CommandRevisions {
		if b, err := hex.DecodeString(revision); err != nil || len(b) != 32 {
			return out, fmt.Errorf("invalid command revision")
		}
	}
	if r.CommandRevisions == nil {
		r.CommandRevisions = []string{}
	}
	if !slices.Contains(r.Scopes, "inspect") {
		return out, fmt.Errorf("inspect scope is required for connection discovery")
	}
	if r.SecretRefs == nil {
		r.SecretRefs = []string{}
	}
	for _, name := range r.SecretRefs {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`).MatchString(name) {
			return out, fmt.Errorf("invalid secret reference name")
		}
	}
	now := a.Now()
	out.Token = "ding_mcp_" + integrationID()
	out.Grant = store.IntegrationGrant{ID: integrationID(), Name: r.Name, Scopes: r.Scopes, CommandRevisions: r.CommandRevisions, SecretRefs: r.SecretRefs, CreatedAt: now, ExpiresAt: now.Add(time.Duration(r.Days) * 24 * time.Hour)}
	err := a.Store.Update(ctx, func(tx *store.Tx) error { return tx.CreateIntegrationGrant(out.Grant, IntegrationTokenHash(out.Token)) })
	return out, err
}

func (a *App) IntegrationAccess(ctx context.Context, token, scope string) (store.IntegrationGrant, error) {
	var grant store.IntegrationGrant
	if len(token) != 73 {
		return grant, store.ErrIntegrationDenied
	}
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		grant, err = tx.IntegrationGrantByHash(IntegrationTokenHash(token))
		if err != nil {
			return err
		}
		if !grant.Allows(scope, a.Now()) {
			return store.ErrIntegrationDenied
		}
		return nil
	})
	return grant, err
}

func requireIntegration(tx *store.Tx, id, scope string, now time.Time) (store.IntegrationGrant, error) {
	g, err := tx.IntegrationGrant(id)
	if err != nil {
		return g, err
	}
	if !g.Allows(scope, now) {
		return g, store.ErrIntegrationDenied
	}
	return g, nil
}
func commandAccess(g store.IntegrationGrant, b plan.Bundle) error {
	check := func(ref *watch.SecretRef) error {
		if ref != nil && !slices.Contains(g.SecretRefs, ref.Env) {
			return fmt.Errorf("%w: secret reference requires local permission", store.ErrIntegrationDenied)
		}
		return nil
	}
	for _, w := range b.Watches {
		if w.Definition.Spec.Source.Type == "command" && !slices.Contains(g.CommandRevisions, w.Revision) {
			return fmt.Errorf("%w: command watch revision must be explicitly allowed by the local administrator", store.ErrIntegrationDenied)
		}
		s := w.Definition.Spec.Source
		if err := check(s.URLRef); err != nil {
			return err
		}
		for _, refs := range []map[string]watch.SecretRef{s.Headers, s.Env} {
			for _, ref := range refs {
				if err := check(&ref); err != nil {
					return err
				}
			}
		}
	}
	for _, d := range b.Destinations {
		if err := check(d.Definition.Spec.URLRef); err != nil {
			return err
		}
		for _, ref := range d.Definition.Spec.Headers {
			if err := check(&ref); err != nil {
				return err
			}
		}
	}
	return nil
}

type IntegrationPreview struct {
	Handle    string      `json:"handle"`
	ExpiresAt time.Time   `json:"expiresAt"`
	Changes   ApplyResult `json:"changes"`
}

func (a *App) PreviewIntegration(ctx context.Context, grant, manifest string) (IntegrationPreview, error) {
	var out IntegrationPreview
	if len(manifest) > 1<<20 {
		return out, fmt.Errorf("manifest exceeds 1 MiB")
	}
	b, err := plan.Parse([]byte(manifest))
	if err != nil {
		return out, err
	}
	out.Changes, err = a.Apply(ctx, ApplyRequest{Manifest: manifest, DryRun: true})
	if err != nil {
		return out, err
	}
	out.Handle = integrationID()
	out.ExpiresAt = a.Now().Add(15 * time.Minute)
	review, err := json.Marshal(out.Changes.Review)
	if err != nil {
		return out, err
	}
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		g, err := requireIntegration(tx, grant, "preview", a.Now())
		if err != nil {
			return err
		}
		if err = commandAccess(g, b); err != nil {
			return err
		}
		return tx.SaveIntegrationPreview(store.IntegrationPreview{ID: out.Handle, GrantID: grant, Manifest: manifest, Review: review, ExpiresAt: out.ExpiresAt}, a.Now())
	})
	return out, err
}

// Receipt lookup and authorization run inside the same transaction as the effect.
// The key namespace spans all actions, so reusing a key for another action fails.
type integrationMutation struct {
	grant, key, action, digest, scope string
	check                             func(*store.Tx, store.IntegrationGrant) error
}

var operationKey = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)

func newIntegrationMutation(grant, key, action, scope string, args any) (*integrationMutation, error) {
	if !operationKey.MatchString(key) {
		return nil, fmt.Errorf("operation key must be 16–128 letters, digits, underscores or hyphens; generate a UUID once per intended action")
	}
	digest, err := watch.Revision(args)
	if err != nil {
		return nil, err
	}
	return &integrationMutation{grant: grant, key: key, action: action, digest: digest, scope: scope}, nil
}
func (m *integrationMutation) begin(tx *store.Tx, result any, now time.Time) (bool, error) {
	if m == nil {
		return false, nil
	}
	g, err := requireIntegration(tx, m.grant, m.scope, now)
	if err != nil {
		return false, err
	}
	o, err := tx.IntegrationOperation(m.grant, m.key)
	if err == nil {
		if o.Action != m.action || o.Digest != m.digest {
			return false, store.ErrOperationConflict
		}
		return true, json.Unmarshal(o.Result, result)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	if m.check != nil {
		return false, m.check(tx, g)
	}
	return false, nil
}
func (m *integrationMutation) finish(tx *store.Tx, result any, now time.Time) error {
	if m == nil {
		return nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.SaveIntegrationOperation(m.grant, store.IntegrationOperation{Key: m.key, Action: m.action, Digest: m.digest, Result: raw, CreatedAt: now})
}

func (a *App) ApplyIntegration(ctx context.Context, grant, handle, key string) (ApplyResult, error) {
	var result ApplyResult
	var request ApplyRequest
	m, err := newIntegrationMutation(grant, key, "apply", "manage", handle)
	if err != nil {
		return result, err
	}
	var done bool
	// Reconcile a committed retry before looking up a possibly expired preview.
	err = a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		done, err = m.begin(tx, &result, a.Now())
		if done || err != nil {
			return err
		}
		p, err := tx.IntegrationPreview(grant, handle, a.Now())
		if err != nil {
			return err
		}
		request.Manifest = p.Manifest
		return json.Unmarshal(p.Review, &request.Review)
	})
	if done || err != nil {
		return result, err
	}
	m.check = func(tx *store.Tx, g store.IntegrationGrant) error {
		if _, err := tx.IntegrationPreview(grant, handle, a.Now()); err != nil {
			return err
		}
		b, err := plan.Parse([]byte(request.Manifest))
		if err != nil {
			return err
		}
		return commandAccess(g, b)
	}
	return a.apply(ctx, request, m)
}

func (a *App) LifecycleIntegration(ctx context.Context, grant, id, key string, r LifecycleRequest) (store.WatchRecord, error) {
	if r.Expected == "" || r.ExpectedGeneration == nil {
		return store.WatchRecord{}, fmt.Errorf("expected revision and generation are required")
	}
	m, err := newIntegrationMutation(grant, key, r.Action, "manage", struct {
		ID      string
		Request LifecycleRequest
	}{id, r})
	if err != nil {
		return store.WatchRecord{}, err
	}
	m.check = func(tx *store.Tx, g store.IntegrationGrant) error {
		if r.Action != "resume" {
			return nil
		}
		w, err := tx.Watch(id)
		if err != nil {
			return err
		}
		return commandAccess(g, plan.Bundle{Watches: []plan.Compiled{w.Plan}})
	}
	return a.lifecycle(ctx, id, r, m)
}
func (a *App) RetryIntegration(ctx context.Context, grant string, id int64, key string) error {
	m, err := newIntegrationMutation(grant, key, "retry", "retry", id)
	if err != nil {
		return err
	}
	return a.retry(ctx, id, m)
}
func (a *App) GetIntegrationOperation(ctx context.Context, grant, key string) (store.IntegrationOperation, error) {
	var out store.IntegrationOperation
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		g, err := tx.IntegrationGrant(grant)
		if err != nil {
			return err
		}
		out, err = tx.IntegrationOperation(grant, key)
		if err != nil {
			return err
		}
		scope := "manage"
		if out.Action == "retry" {
			scope = "retry"
		}
		if !g.Allows(scope, a.Now()) {
			return store.ErrIntegrationDenied
		}
		return nil
	})
	return out, err
}
