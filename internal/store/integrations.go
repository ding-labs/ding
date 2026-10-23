package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

var ErrIntegrationDenied = errors.New("integration access denied")
var ErrIntegrationLimit = errors.New("integration storage limit reached")
var ErrPreviewExpired = errors.New("preview expired; preview the changes again")
var ErrOperationConflict = errors.New("operation key already used for a different request")

// Grants apply to the entire instance. Command execution additionally requires an
// exact compiled watch revision authorized by the local administrator.
type IntegrationGrant struct {
	ProviderConsent  string    `json:"providerConsent,omitempty"`
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Scopes           []string  `json:"scopes"`
	CommandRevisions []string  `json:"commandRevisions"`
	SecretRefs       []string  `json:"secretRefs"`
	CreatedAt        time.Time `json:"createdAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Revoked          bool      `json:"revoked"`
}

func (g IntegrationGrant) Allows(scope string, now time.Time) bool {
	return !g.Revoked && g.ExpiresAt.After(now) && slices.Contains(g.Scopes, scope)
}

func (t *Tx) CreateIntegrationGrant(g IntegrationGrant, hash string) error {
	var count int
	if err := t.sql.QueryRowContext(t.ctx, t.query(qCreateIntegrationGrant1)).Scan(&count); err != nil {
		return err
	}
	if count >= 256 {
		return ErrIntegrationLimit
	}
	b, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = t.exec(qCreateIntegrationGrant2, g.ID, hash, b)
	return err
}

func (t *Tx) IntegrationGrant(id string) (IntegrationGrant, error) {
	var g IntegrationGrant
	var raw []byte
	err := t.sql.QueryRowContext(t.ctx, t.query(qIntegrationGrant1), id).Scan(&raw, &g.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrIntegrationDenied
	}
	if err != nil {
		return g, err
	}
	revoked := g.Revoked
	err = json.Unmarshal(raw, &g)
	g.Revoked = revoked
	return g, err
}

func (t *Tx) IntegrationGrantByHash(hash string) (IntegrationGrant, error) {
	var id string
	err := t.sql.QueryRowContext(t.ctx, t.query(qIntegrationGrantByHash1), hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return IntegrationGrant{}, ErrIntegrationDenied
	}
	if err != nil {
		return IntegrationGrant{}, err
	}
	return t.IntegrationGrant(id)
}

func (t *Tx) IntegrationGrants() ([]IntegrationGrant, error) {
	rows, err := t.sql.QueryContext(t.ctx, t.query(qIntegrationGrants1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []IntegrationGrant{}
	for rows.Next() {
		var raw []byte
		var revoked bool
		var g IntegrationGrant
		if err = rows.Scan(&raw, &revoked); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &g); err != nil {
			return nil, err
		}
		g.Revoked = revoked
		result = append(result, g)
	}
	return result, rows.Err()
}

// RenewProviderIntegrationGrant extends a private grant only after the caller
// verifies a fresh provider token and current membership. Revocation is final
// for this provider consent, even if its JWT or refresh token remains usable.
func (t *Tx) RenewProviderIntegrationGrant(id, consent string, scopes, refs []string, until time.Time) error {
	g, err := t.IntegrationGrant(id)
	if err != nil {
		return err
	}
	if consent == "" || g.ProviderConsent != consent || g.Revoked {
		return ErrIntegrationDenied
	}
	if g.ExpiresAt.After(until.Add(-24*time.Hour)) && slices.Equal(g.Scopes, scopes) && slices.Equal(g.SecretRefs, refs) {
		return nil
	}
	g.Scopes, g.SecretRefs, g.ExpiresAt = scopes, refs, until
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = t.exec(qRenewProviderIntegrationGrant1, raw, id)
	return err
}

func (t *Tx) RevokeIntegrationGrant(id string) error {
	r, err := t.exec(qRevokeIntegrationGrant1, id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

type IntegrationPreview struct {
	ID        string          `json:"id"`
	GrantID   string          `json:"-"`
	Manifest  string          `json:"-"`
	Review    json.RawMessage `json:"-"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

func (t *Tx) SaveIntegrationPreview(p IntegrationPreview, now time.Time) error {
	if _, err := t.exec(qSaveIntegrationPreview1, timestamp(now)); err != nil {
		return err
	}
	var count int
	if err := t.sql.QueryRowContext(t.ctx, t.query(qSaveIntegrationPreview2)).Scan(&count); err != nil {
		return err
	}
	if count >= 64 {
		return ErrIntegrationLimit
	}
	// Bind bytes for BLOB/bytea storage; pgx otherwise sends json.RawMessage
	// as JSON text, which PostgreSQL would parse for bytea escapes.
	_, err := t.exec(qSaveIntegrationPreview3, p.ID, p.GrantID, p.Manifest, []byte(p.Review), timestamp(p.ExpiresAt))
	return err
}

func (t *Tx) IntegrationPreview(grant, id string, now time.Time) (IntegrationPreview, error) {
	p := IntegrationPreview{ID: id, GrantID: grant}
	var at int64
	err := t.sql.QueryRowContext(t.ctx, t.query(qIntegrationPreview1), id, grant).Scan(&p.Manifest, &p.Review, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrPreviewExpired
	}
	if err != nil {
		return p, err
	}
	p.ExpiresAt = instant(at)
	if !p.ExpiresAt.After(now) {
		return p, ErrPreviewExpired
	}
	return p, nil
}

type IntegrationOperation struct {
	Key       string          `json:"key"`
	Action    string          `json:"action"`
	Digest    string          `json:"-"`
	Result    json.RawMessage `json:"result"`
	CreatedAt time.Time       `json:"createdAt"`
}

func (t *Tx) IntegrationOperation(grant, key string) (IntegrationOperation, error) {
	o := IntegrationOperation{Key: key}
	var at int64
	err := t.sql.QueryRowContext(t.ctx, t.query(qIntegrationOperation1), grant, key).Scan(&o.Action, &o.Digest, &o.Result, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNotFound
	}
	o.CreatedAt = instant(at)
	return o, err
}

func (t *Tx) SaveIntegrationOperation(grant string, o IntegrationOperation) error {
	// Admission control is inside the mutation transaction: hitting this bound
	// rolls back the action rather than committing without a durable receipt.
	var count int
	if err := t.sql.QueryRowContext(t.ctx, t.query(qSaveIntegrationOperation1)).Scan(&count); err != nil {
		return err
	}
	if count >= 10000 {
		return ErrIntegrationLimit
	}
	_, err := t.exec(qSaveIntegrationOperation2, grant, o.Key, o.Action, o.Digest, []byte(o.Result), timestamp(o.CreatedAt))
	return err
}
