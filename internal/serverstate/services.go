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
	LastUsedAt *time.Time        `json:"lastUsedAt,omitempty"`
	Lifetimes  map[string]string `json:"-"`
	KeyID      string            `json:"keyId,omitempty"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Workspace  string            `json:"workspace"`
	CreatedBy  string            `json:"createdBy"`
	Scopes     []string          `json:"scopes"`
	Watches    []string          `json:"watches"`
	ExpiresAt  time.Time         `json:"expiresAt"`
	Token      string            `json:"token,omitempty"`
}

func (d *DB) CreateService(ctx context.Context, p access.Principal, name string, scopes, watches []string, days int) (Service, error) {
	return d.CreateBoundService(ctx, p, name, scopes, watches, days, nil)
}
func (d *DB) CreateBoundService(ctx context.Context, p access.Principal, name string, scopes, watches []string, days int, lifetimes map[string]string) (Service, error) {
	var out Service
	if err := validateService(name, scopes, watches, days); err != nil {
		return out, err
	}

	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, tx.query(qCreateBoundService1), time.Now().Unix()); err != nil {
		return out, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, tx.query(qCreateBoundService2), p.Workspace).Scan(&n); err != nil {
		return out, err
	}
	if n >= 100 {
		return out, fmt.Errorf("workspace service limit reached")
	}
	out = Service{Lifetimes: lifetimes, KeyID: ID(), ID: ID(), Name: name, Workspace: p.Workspace, CreatedBy: p.User, Scopes: scopes, Watches: watches, ExpiresAt: time.Now().Add(time.Duration(days) * 24 * time.Hour), Token: "ding_service_" + ID()}
	rawScopes, _ := json.Marshal(scopes)
	rawWatches, _ := json.Marshal(watches)
	rawLifetimes, _ := json.Marshal(lifetimes)
	if _, err = tx.ExecContext(ctx, tx.query(qCreateBoundService3), out.ID, TokenHash(out.Token), name, p.Workspace, p.User, string(rawScopes), string(rawWatches), out.ExpiresAt.Unix(), string(rawLifetimes)); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, tx.query(qCreateBoundService4), out.KeyID, out.ID, TokenHash(out.Token), out.ExpiresAt.Unix(), time.Now().UnixNano()); err != nil {
		return out, err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:create", out.ID); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (d *DB) Services(ctx context.Context, workspace string) ([]Service, error) {
	rows, err := d.sql.QueryContext(ctx, d.query(qServices1), workspace)
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
func scanService(row interface{ Scan(...any) error }, extra ...any) (Service, error) {
	var s Service
	var scopes, watches, lifetimes string
	var at int64
	err := row.Scan(append([]any{&s.ID, &s.Name, &s.Workspace, &s.CreatedBy, &scopes, &watches, &at, &lifetimes}, extra...)...)
	if err == nil {
		err = json.Unmarshal([]byte(scopes), &s.Scopes)
	}
	if err == nil {
		err = json.Unmarshal([]byte(watches), &s.Watches)
	}
	if err == nil {
		err = json.Unmarshal([]byte(lifetimes), &s.Lifetimes)
	}
	s.ExpiresAt = time.Unix(at, 0)
	return s, err
}
func (d *DB) ServiceToken(ctx context.Context, token string) (Service, error) {
	if len(token) != len("ding_service_")+64 {
		return Service{}, fmt.Errorf("invalid service credential")
	}
	var keyID string
	var used int64
	out, err := scanService(d.credentialDB().QueryRowContext(ctx, d.query(qServiceToken1), TokenHash(token), time.Now().Unix()), &keyID, &used)
	out.KeyID = keyID
	if used > 0 {
		at := time.Unix(used, 0).UTC()
		out.LastUsedAt = &at
	}
	return out, err
}
func (d *DB) RevokeService(ctx context.Context, p access.Principal, id string) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return err
	}
	if err = tx.lockService(ctx, p.Workspace, id); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, tx.query(qRevokeService1), p.Workspace, id); err != nil {
		return err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "service:revoke", id); err != nil {
		return err
	}
	return tx.Commit()
}

func validateService(name string, scopes, watches []string, days int) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 || days < 1 || days > 90 || len(scopes) == 0 || len(scopes) > 3 || len(watches) > 100 {
		return fmt.Errorf("provide name, bounded scopes, and expiry of 1–90 days")
	}
	for _, scope := range scopes {
		if !slices.Contains([]string{"inspect", "retry", "ingest"}, scope) {
			return fmt.Errorf("service scope is not supported")
		}
	}
	if slices.Contains(scopes, "ingest") && len(watches) == 0 {
		return fmt.Errorf("ingest requires explicit watch IDs")
	}
	for _, id := range watches {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\") {
			return fmt.Errorf("invalid watch ID")
		}
	}
	return nil
}

// NoteServiceUse is diagnostic: failure never changes an already committed
// acceptance. Callers skip fresh keys; the predicate coalesces concurrent uses.
func (d *DB) NoteServiceUse(ctx context.Context, s Service) {
	now := time.Now()
	if s.KeyID == "" || (s.LastUsedAt != nil && now.Sub(*s.LastUsedAt) < time.Minute) {
		return
	}
	_, _ = d.exec(ctx, qNoteServiceUse1, now.Unix(), s.KeyID, now.Add(-time.Minute).Unix())
}
