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
	if err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM integration_grants").Scan(&count); err != nil {
		return err
	}
	if count >= 256 {
		return ErrIntegrationLimit
	}
	b, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, "INSERT INTO integration_grants(id,token_hash,body) VALUES(?,?,?)", g.ID, hash, b)
	return err
}

func (t *Tx) IntegrationGrant(id string) (IntegrationGrant, error) {
	var g IntegrationGrant
	var raw []byte
	err := t.sql.QueryRowContext(t.ctx, "SELECT body,revoked FROM integration_grants WHERE id=?", id).Scan(&raw, &g.Revoked)
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
	err := t.sql.QueryRowContext(t.ctx, "SELECT id FROM integration_grants WHERE token_hash=?", hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return IntegrationGrant{}, ErrIntegrationDenied
	}
	if err != nil {
		return IntegrationGrant{}, err
	}
	return t.IntegrationGrant(id)
}

func (t *Tx) IntegrationGrants() ([]IntegrationGrant, error) {
	rows, err := t.sql.QueryContext(t.ctx, "SELECT body,revoked FROM integration_grants ORDER BY id")
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

func (t *Tx) RevokeIntegrationGrant(id string) error {
	r, err := t.sql.ExecContext(t.ctx, "UPDATE integration_grants SET revoked=1 WHERE id=?", id)
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
	if _, err := t.sql.ExecContext(t.ctx, "DELETE FROM integration_previews WHERE expires_at<=?", timestamp(now)); err != nil {
		return err
	}
	var count int
	if err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM integration_previews").Scan(&count); err != nil {
		return err
	}
	if count >= 64 {
		return ErrIntegrationLimit
	}
	_, err := t.sql.ExecContext(t.ctx, "INSERT INTO integration_previews(id,grant_id,manifest,review,expires_at) VALUES(?,?,?,?,?)", p.ID, p.GrantID, p.Manifest, p.Review, timestamp(p.ExpiresAt))
	return err
}

func (t *Tx) IntegrationPreview(grant, id string, now time.Time) (IntegrationPreview, error) {
	p := IntegrationPreview{ID: id, GrantID: grant}
	var at int64
	err := t.sql.QueryRowContext(t.ctx, "SELECT manifest,review,expires_at FROM integration_previews WHERE id=? AND grant_id=?", id, grant).Scan(&p.Manifest, &p.Review, &at)
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
	err := t.sql.QueryRowContext(t.ctx, "SELECT action,digest,result,created_at FROM integration_operations WHERE grant_id=? AND operation_key=?", grant, key).Scan(&o.Action, &o.Digest, &o.Result, &at)
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
	if err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM integration_operations").Scan(&count); err != nil {
		return err
	}
	if count >= 10000 {
		return ErrIntegrationLimit
	}
	_, err := t.sql.ExecContext(t.ctx, "INSERT INTO integration_operations(grant_id,operation_key,action,digest,result,created_at) VALUES(?,?,?,?,?,?)", grant, o.Key, o.Action, o.Digest, o.Result, timestamp(o.CreatedAt))
	return err
}
