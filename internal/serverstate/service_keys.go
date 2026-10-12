package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

// RotateService keeps the logical producer and its retry namespace. At most
// two keys remain: the new key and the immediately previous overlap key.
func (d *DB) RotateService(ctx context.Context, p access.Principal, id string, days, overlapSeconds int) (Service, error) {
	var out Service
	if days < 1 || days > 90 || overlapSeconds < 0 || overlapSeconds > 86400 {
		return out, fmt.Errorf("expiry must be 1–90 days and overlap 0–86400 seconds")
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return out, err
	}
	out, err = scanService(tx.QueryRowContext(ctx, "SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM service_grants WHERE workspace=? AND id=?", p.Workspace, id))
	if err != nil {
		return out, err
	}
	now := time.Now()
	// Delete older keys before shortening the current key's overlap window.
	if _, err = tx.ExecContext(ctx, "DELETE FROM service_keys WHERE service=? AND id NOT IN (SELECT id FROM service_keys WHERE service=? ORDER BY created_at DESC,id DESC LIMIT 1)", id, id); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE service_keys SET expires_at=min(expires_at,?) WHERE service=?", now.Add(time.Duration(overlapSeconds)*time.Second).Unix(), id); err != nil {
		return out, err
	}
	out.KeyID = ID()
	out.Token = "ding_service_" + ID()
	out.ExpiresAt = now.Add(time.Duration(days) * 24 * time.Hour)
	if _, err = tx.ExecContext(ctx, "INSERT INTO service_keys(id,service,hash,expires_at,created_at) VALUES(?,?,?,?,?)", out.KeyID, id, TokenHash(out.Token), out.ExpiresAt.Unix(), now.UnixNano()); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE service_grants SET expires_at=?,hash=? WHERE id=?", out.ExpiresAt.Unix(), TokenHash(out.Token), id); err != nil {
		return out, err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:rotate", id); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

type ServiceKey struct {
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	ID         string     `json:"id"`
	ExpiresAt  time.Time  `json:"expiresAt"`
}

func (d *DB) ServiceKeys(ctx context.Context, workspace, id string) ([]ServiceKey, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT k.id,k.expires_at,k.last_used_at FROM service_keys k JOIN service_grants s ON s.id=k.service WHERE s.workspace=? AND s.id=? ORDER BY k.created_at DESC", workspace, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceKey{}
	for rows.Next() {
		var key ServiceKey
		var at, used int64
		if err = rows.Scan(&key.ID, &at, &used); err != nil {
			return nil, err
		}
		key.ExpiresAt = time.Unix(at, 0)
		if used > 0 {
			last := time.Unix(used, 0).UTC()
			key.LastUsedAt = &last
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (d *DB) RevokeServiceKey(ctx context.Context, p access.Principal, id, key string) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM service_keys WHERE id=? AND service=? AND service IN (SELECT id FROM service_grants WHERE workspace=?)", key, id, p.Workspace); err != nil {
		return err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:key-revoke", key); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateService changes the resource grant under the same fence as revocation.
// Rotating keys and receipts continue to refer to the logical producer.
func (d *DB) UpdateService(ctx context.Context, p access.Principal, id, name string, scopes, watches []string, lifetimes map[string]string) (Service, error) {
	var out Service
	if err := validateService(name, scopes, watches, 1); err != nil {
		return out, err
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return out, err
	}
	rawScopes, _ := json.Marshal(scopes)
	rawWatches, _ := json.Marshal(watches)
	rawLifetimes, _ := json.Marshal(lifetimes)
	result, err := tx.ExecContext(ctx, "UPDATE service_grants SET name=?,scopes=?,watches=?,lifetimes=? WHERE id=? AND workspace=?", name, string(rawScopes), string(rawWatches), string(rawLifetimes), id, p.Workspace)
	if err != nil {
		return out, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return out, fmt.Errorf("producer unavailable")
	}
	out, err = scanService(tx.QueryRowContext(ctx, "SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM service_grants WHERE id=? AND workspace=?", id, p.Workspace))
	if err != nil {
		return out, err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:update", id); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
