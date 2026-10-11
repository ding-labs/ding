package state

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/access"
	"slices"
	"strings"
	"time"
)

type Service struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Workspace string    `json:"workspace"`
	CreatedBy string    `json:"createdBy"`
	Scopes    []string  `json:"scopes"`
	Watches   []string  `json:"watches"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"token,omitempty"`
}

func (d *DB) CreateService(ctx context.Context, p access.Principal, name string, scopes, watches []string, days int) (Service, error) {
	var out Service
	if strings.TrimSpace(name) == "" || len(name) > 100 || days < 1 || days > 90 || len(scopes) == 0 || len(scopes) > 3 || len(watches) > 100 {
		return out, fmt.Errorf("provide name, bounded scopes, and expiry of 1–90 days")
	}
	for _, scope := range scopes {
		if !slices.Contains([]string{"inspect", "retry", "ingest"}, scope) {
			return out, fmt.Errorf("service scope is not supported")
		}
	}
	if slices.Contains(scopes, "ingest") && len(watches) == 0 {
		return out, fmt.Errorf("ingest requires explicit watch IDs")
	}
	for _, id := range watches {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\") {
			return out, fmt.Errorf("invalid watch ID")
		}
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
	if _, err = tx.ExecContext(ctx, "DELETE FROM service_grants WHERE expires_at<=?", time.Now().Unix()); err != nil {
		return out, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM service_grants WHERE workspace=?", p.Workspace).Scan(&n); err != nil {
		return out, err
	}
	if n >= 100 {
		return out, fmt.Errorf("workspace service limit reached")
	}
	out = Service{ID: ID(), Name: name, Workspace: p.Workspace, CreatedBy: p.User, Scopes: scopes, Watches: watches, ExpiresAt: time.Now().Add(time.Duration(days) * 24 * time.Hour), Token: "ding_service_" + ID()}
	rawScopes, _ := json.Marshal(scopes)
	rawWatches, _ := json.Marshal(watches)
	if _, err = tx.ExecContext(ctx, "INSERT INTO service_grants(id,hash,name,workspace,created_by,scopes,watches,expires_at) VALUES(?,?,?,?,?,?,?,?)", out.ID, TokenHash(out.Token), name, p.Workspace, p.User, string(rawScopes), string(rawWatches), out.ExpiresAt.Unix()); err != nil {
		return out, err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:create", out.ID); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (d *DB) Services(ctx context.Context, workspace string) ([]Service, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT id,name,workspace,created_by,scopes,watches,expires_at FROM service_grants WHERE workspace=? ORDER BY id LIMIT 100", workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Service{}
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func scanService(row interface{ Scan(...any) error }) (Service, error) {
	var s Service
	var scopes, watches string
	var at int64
	err := row.Scan(&s.ID, &s.Name, &s.Workspace, &s.CreatedBy, &scopes, &watches, &at)
	if err == nil {
		err = json.Unmarshal([]byte(scopes), &s.Scopes)
	}
	if err == nil {
		err = json.Unmarshal([]byte(watches), &s.Watches)
	}
	s.ExpiresAt = time.Unix(at, 0)
	return s, err
}
func (d *DB) ServiceToken(ctx context.Context, token string) (Service, error) {
	if len(token) != len("ding_service_")+64 {
		return Service{}, fmt.Errorf("invalid service credential")
	}
	return scanService(d.sql.QueryRowContext(ctx, "SELECT id,name,workspace,created_by,scopes,watches,expires_at FROM service_grants WHERE hash=? AND expires_at>? AND workspace IN (SELECT id FROM workspaces WHERE deleting=0)", TokenHash(token), time.Now().Unix()))
}
func (d *DB) RevokeService(ctx context.Context, p access.Principal, id string) error {
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
	if _, err = tx.ExecContext(ctx, "DELETE FROM service_grants WHERE workspace=? AND id=?", p.Workspace, id); err != nil {
		return err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:revoke", id); err != nil {
		return err
	}
	return tx.Commit()
}
