package state

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/database"
)

type WebhookConnection struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Workspace      string    `json:"workspace"`
	Provider       string    `json:"provider"`
	Watch          string    `json:"watch"`
	Lifetime       string    `json:"-"`
	Repositories   []string  `json:"repositories,omitempty"`
	Account        string    `json:"account,omitempty"`
	Live           bool      `json:"live"`
	Events         []string  `json:"events,omitempty"`
	Secret         string    `json:"-"`
	PreviousSecret string    `json:"-"`
	PreviousUntil  time.Time `json:"previousUntil,omitempty"`
}

func (v *Vault) CreateWebhook(ctx context.Context, p access.Principal, c WebhookConnection, secret string) (WebhookConnection, error) {
	if c.Name == "" || len(c.Name) > 100 || c.Watch == "" || c.Lifetime == "" || len(secret) < 16 || len(secret) > 4096 || !slices.Contains([]string{"github", "stripe", "workos"}, c.Provider) {
		return c, fmt.Errorf("provide a name, provider, existing push watch and a strong signing secret")
	}
	if len(c.Repositories) > 100 || len(c.Events) > 100 {
		return c, fmt.Errorf("connection resource limit exceeded")
	}
	if c.Provider == "github" && len(c.Repositories) == 0 {
		return c, fmt.Errorf("GitHub requires explicit immutable repository IDs")
	}
	if c.Provider == "stripe" && (c.Account == "" || len(c.Events) == 0) {
		return c, fmt.Errorf("Stripe requires its account ID and explicit snapshot event types")
	}
	if c.Provider == "workos" && (len(c.Events) != 1 || c.Events[0] != "user.created" || len(c.Repositories) != 0 || c.Account != "" || c.Live) {
		return c, fmt.Errorf("WorkOS connections require only the user.created event; the environment is bound by its signing secret")
	}
	for _, id := range c.Repositories {
		if id == "" || len(id) > 32 {
			return c, fmt.Errorf("invalid repository ID")
		}
	}
	for _, event := range c.Events {
		if event == "" || len(event) > 128 {
			return c, fmt.Errorf("invalid event type")
		}
	}
	c.ID = ID()
	c.Workspace = p.Workspace
	cipher, err := v.aead(c.Workspace)
	if err != nil {
		return c, err
	}
	encrypted := cipher.Seal(nil, nil, []byte(secret), []byte("webhook:"+c.ID))
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	v.db.Access.Lock()
	defer v.db.Access.Unlock()
	tx, err := v.db.begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return c, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, tx.query(qCreateWebhook1), p.Workspace).Scan(&n); err != nil {
		return c, err
	}
	if n >= 100 {
		return c, fmt.Errorf("connection capacity reached")
	}
	if _, err = tx.ExecContext(ctx, tx.query(qCreateWebhook2), c.ID, c.Workspace, c.Lifetime, raw, encrypted); err != nil {
		return c, err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "webhook:create", c.ID); err != nil {
		return c, err
	}
	return c, tx.Commit()
}
func (v *Vault) Webhook(ctx context.Context, id string) (WebhookConnection, error) {
	var c WebhookConnection
	var raw, secret, previous []byte
	var until int64
	err := database.Executor(ctx, v.db.sql).QueryRowContext(ctx, v.db.query(qWebhook1), id).Scan(&raw, &c.Lifetime, &secret, &previous, &until)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	cipher, err := v.aead(c.Workspace)
	if err != nil {
		return c, err
	}
	plain, err := cipher.Open(nil, nil, secret, []byte("webhook:"+c.ID))
	if err != nil {
		return c, fmt.Errorf("webhook secret unavailable")
	}
	c.Secret = string(plain)
	c.PreviousUntil = time.Unix(until, 0)
	if len(previous) > 0 && time.Now().Before(c.PreviousUntil) {
		plain, err = cipher.Open(nil, nil, previous, []byte("webhook:"+c.ID))
		if err != nil {
			return c, fmt.Errorf("webhook secret unavailable")
		}
		c.PreviousSecret = string(plain)
	}
	return c, nil
}
func (d *DB) Webhooks(ctx context.Context, workspace string) ([]WebhookConnection, error) {
	rows, err := d.sql.QueryContext(ctx, d.query(qWebhooks1), workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookConnection{}
	for rows.Next() {
		var raw []byte
		var c WebhookConnection
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (v *Vault) RotateWebhook(ctx context.Context, p access.Principal, id, secret string, overlap int) error {
	if len(secret) < 16 || len(secret) > 4096 || overlap < 0 || overlap > 86400 {
		return fmt.Errorf("invalid signing secret or overlap")
	}
	cipher, err := v.aead(p.Workspace)
	if err != nil {
		return err
	}
	encrypted := cipher.Seal(nil, nil, []byte(secret), []byte("webhook:"+id))
	v.db.Access.Lock()
	defer v.db.Access.Unlock()
	tx, err := v.db.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err = memberActor(ctx, tx, p)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.query(qRotateWebhook1), time.Now().Add(time.Duration(overlap)*time.Second).Unix(), encrypted, id, p.Workspace)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("connection unavailable")
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "webhook:rotate", id); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) DeleteWebhook(ctx context.Context, p access.Principal, id string) error {
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
	if _, err = tx.ExecContext(ctx, tx.query(qDeleteWebhook1), id, p.Workspace); err != nil {
		return err
	}
	if err = audit(ctx, tx, p.User, p.Workspace, "webhook:delete", id); err != nil {
		return err
	}
	return tx.Commit()
}
