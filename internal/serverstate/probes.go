package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"
)

type Probe struct {
	ID      string    `json:"id"`
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
}

var operationID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func (v *Vault) Revision(ctx context.Context, account, name string) (string, error) {
	var data []byte
	err := v.db.sql.QueryRowContext(ctx, "SELECT ciphertext FROM secrets WHERE account=? AND name=?", account, name).Scan(&data)
	return TokenHash(string(data)), err
}

// BeginProbe reserves an external test before I/O. Pending is an uncertain
// outcome after process loss; replaying a key never sends a second notification.
func (d *DB) BeginProbe(ctx context.Context, account, id, digest string) (Probe, bool, error) {
	p := Probe{ID: id, Outcome: "pending", At: time.Now().UTC()}
	if !operationID.MatchString(id) || len(digest) != 64 {
		return p, false, fmt.Errorf("invalid probe identity")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return p, false, err
	}
	defer tx.Rollback()
	var old string
	var at int64
	err = tx.QueryRowContext(ctx, "SELECT digest,outcome,at FROM probes WHERE account=? AND id=?", account, id).Scan(&old, &p.Outcome, &at)
	if err == nil {
		if old != digest {
			return p, false, fmt.Errorf("operation key was already used for a different test or credential revision")
		}
		p.At = time.Unix(at, 0).UTC()
		return p, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, false, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM probes WHERE account=?", account).Scan(&count); err != nil {
		return p, false, err
	}
	if count >= 1000 {
		return p, false, fmt.Errorf("workspace test-receipt quota reached")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO probes(account,id,digest,outcome,at) VALUES(?,?,?,?,?)", account, id, digest, p.Outcome, p.At.Unix()); err != nil {
		return p, false, err
	}
	return p, true, tx.Commit()
}

func (d *DB) FinishProbe(ctx context.Context, account, id, outcome string) error {
	if outcome != "accepted" && outcome != "failed" {
		return fmt.Errorf("invalid probe outcome")
	}
	result, err := d.sql.ExecContext(ctx, "UPDATE probes SET outcome=? WHERE account=? AND id=? AND outcome='pending'", outcome, account, id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("probe is no longer pending")
	}
	return nil
}
