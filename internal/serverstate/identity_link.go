package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
)

type IdentityLink struct {
	User    string `json:"user"`
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

// EnableOIDC is an offline operator operation. It adds a verified immutable
// subject to an existing user and changes the pinned login methods atomically.
// It never matches email, creates a workspace, or changes membership.
func (d *DB) EnableOIDC(ctx context.Context, link IdentityLink) error {
	if !validOIDCIssuer(link.Issuer) || link.Subject == "" || len(link.Subject) > 512 || link.User == "" {
		return fmt.Errorf("provide an exact existing user and verified HTTPS issuer/subject")
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := enableOIDCProvider(ctx, tx, link.Issuer); err != nil {
		return err
	}
	var workspace string
	if err := tx.QueryRowContext(ctx, `SELECT w.id FROM users u JOIN memberships m ON m.user=u.id JOIN workspaces w ON w.id=m.workspace WHERE u.id=? AND u.disabled=0 AND w.deleting=0 ORDER BY w.id LIMIT 1`, link.User).Scan(&workspace); err != nil {
		return fmt.Errorf("existing active user with a workspace required")
	}
	var current string
	err = tx.QueryRowContext(ctx, "SELECT user FROM identities WHERE issuer=? AND subject=?", link.Issuer, link.Subject).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && current != link.User {
		return fmt.Errorf("identity is already linked to another user")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO identities(issuer,subject,user) VALUES(?,?,?) ON CONFLICT(issuer,subject) DO NOTHING", link.Issuer, link.Subject, link.User); err != nil {
		return err
	}
	if err := audit(ctx, tx, "operator", workspace, "identity.link", link.User); err != nil {
		return err
	}
	return tx.Commit()
}

// EnableOIDCLogin enables hosted sign-in without linking or changing existing
// accounts. New identities remain subject to the configured enrollment limit.
// Like EnableOIDC, this is an explicit offline operator operation.
func (d *DB) EnableOIDCLogin(ctx context.Context, issuer string) error {
	if !validOIDCIssuer(issuer) {
		return fmt.Errorf("provide an exact HTTPS issuer")
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := enableOIDCProvider(ctx, tx, issuer); err != nil {
		return err
	}
	if err := audit(ctx, tx, "operator", "", "identity.enable", issuer); err != nil {
		return err
	}
	return tx.Commit()
}

func validOIDCIssuer(issuer string) bool {
	u, err := url.Parse(issuer)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && len(issuer) <= 1024
}

func enableOIDCProvider(ctx context.Context, tx *transaction, issuer string) error {
	var methods, current string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='login_methods'").Scan(&methods); err != nil {
		return err
	}
	if methods != "password" && methods != "oidc,password" {
		return fmt.Errorf("this operation extends an initialized password login profile")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('oidc_issuer',?) ON CONFLICT(key) DO NOTHING", issuer); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='oidc_issuer'").Scan(&current); err != nil {
		return err
	}
	if current != issuer {
		return fmt.Errorf("issuer differs from the pinned identity provider")
	}
	_, err := tx.ExecContext(ctx, "UPDATE settings SET value='oidc,password' WHERE key='login_methods'")
	return err
}

func (d *DB) ConfigureOIDCIssuer(ctx context.Context, issuer string) error {
	if issuer == "" {
		return fmt.Errorf("OIDC issuer required")
	}
	if _, err := d.sql.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('oidc_issuer',?) ON CONFLICT(key) DO NOTHING", issuer); err != nil {
		return err
	}
	var saved string
	if err := d.sql.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='oidc_issuer'").Scan(&saved); err != nil {
		return err
	}
	if saved != issuer {
		return fmt.Errorf("OIDC issuer differs from the initialized instance")
	}
	return nil
}
