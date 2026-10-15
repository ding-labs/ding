package state

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func TokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type Session struct {
	User      string    `json:"user"`
	Scopes    []string  `json:"scopes,omitempty"`
	Workspace string    `json:"workspace"`
	Kind      string    `json:"kind"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"-"`
	CSRF      string    `json:"-"`
	csrfHash  string
}

func (s Session) ValidCSRF(value string) bool {
	return value != "" && subtle.ConstantTimeCompare([]byte(TokenHash(value)), []byte(s.csrfHash)) == 1
}

func (d *DB) NewSession(ctx context.Context, user, workspace, kind string, scopes []string, now time.Time) (Session, error) {
	s := Session{User: user, Scopes: scopes, Workspace: workspace, Kind: kind, ExpiresAt: now.Add(7 * 24 * time.Hour), Token: ID(), CSRF: ID()}
	if kind != "browser" && kind != "device" {
		return s, fmt.Errorf("invalid session kind")
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return s, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, tx.query(qNewSession1), now.Unix()); err != nil {
		return s, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, tx.query(qNewSession2), user).Scan(&count); err != nil {
		return s, err
	}
	if count >= 20 {
		return s, fmt.Errorf("too many active sessions; revoke an existing connection")
	}
	var active int
	if err := tx.QueryRowContext(ctx, tx.query(qNewSession3), user, workspace).Scan(&active); err != nil {
		return s, err
	}
	if active != 1 {
		return s, fmt.Errorf("workspace unavailable")
	}
	if kind == "device" && len(scopes) == 0 {
		return s, fmt.Errorf("device scopes required")
	}
	rawScopes, _ := json.Marshal(scopes)
	s.csrfHash = TokenHash(s.CSRF)
	if _, err := tx.ExecContext(ctx, tx.query(qNewSession4), TokenHash(s.Token), user, workspace, string(rawScopes), s.csrfHash, kind, s.ExpiresAt.Unix()); err != nil {
		return s, err
	}
	if err := audit(ctx, tx, user, workspace, "session:"+kind, TokenHash(s.Token)); err != nil {
		return s, err
	}
	return s, tx.Commit()
}

func (d *DB) Session(ctx context.Context, token string, now time.Time) (Session, error) {
	var s Session
	var expiry int64
	if len(token) != 64 {
		return s, fmt.Errorf("invalid session")
	}
	var raw string
	err := d.sql.QueryRowContext(ctx, d.query(qSession1), TokenHash(token), now.Unix()).Scan(&s.User, &s.Workspace, &raw, &s.Kind, &s.csrfHash, &expiry)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &s.Scopes)
	}
	s.ExpiresAt = time.Unix(expiry, 0).UTC()
	s.Token = token
	return s, err
}

func (d *DB) RevokeSession(ctx context.Context, token string) error {
	_, err := d.exec(ctx, qRevokeSession1, TokenHash(token))
	return err
}

type Login struct{ Verifier, Nonce, ReturnPath string }

func (v *Vault) BeginLogin(ctx context.Context, state string, login Login, now time.Time) error {
	if len(state) != 64 || len(login.Verifier) > 128 || len(login.Nonce) != 64 || len(login.ReturnPath) > 256 {
		return fmt.Errorf("invalid login transaction")
	}
	data, err := json.Marshal(login)
	if err != nil {
		return err
	}
	aead, err := v.aead("_login")
	if err != nil {
		return err
	}
	hash := TokenHash(state)
	encrypted := aead.Seal(nil, nil, data, []byte(hash))
	tx, err := v.db.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, tx.query(qBeginLogin1), now.Unix()); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, tx.query(qBeginLogin2)).Scan(&count); err != nil {
		return err
	}
	if count >= 1000 {
		return fmt.Errorf("login capacity reached; try later")
	}
	if _, err := tx.ExecContext(ctx, tx.query(qBeginLogin3), hash, encrypted, now.Add(10*time.Minute).Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ConsumeLogin deletes the flow in the same transaction that reads it. A lost
// callback starts a new flow rather than replaying an already consumed code.
func (v *Vault) ConsumeLogin(ctx context.Context, state string, now time.Time) (Login, error) {
	var login Login
	var encrypted []byte
	if len(state) != 64 {
		return login, fmt.Errorf("invalid login state")
	}
	hash := TokenHash(state)
	err := v.db.sql.QueryRowContext(ctx, v.db.query(qConsumeLogin1), hash, now.Unix()).Scan(&encrypted)
	if err != nil {
		return login, err
	}
	aead, err := v.aead("_login")
	if err != nil {
		return login, err
	}
	plain, err := aead.Open(nil, nil, encrypted, []byte(hash))
	if err != nil {
		return login, fmt.Errorf("login decryption failed")
	}
	return login, json.Unmarshal(plain, &login)
}
