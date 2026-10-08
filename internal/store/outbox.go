package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
)

type Intent struct {
	ID                  int64                    `json:"id"`
	EventID             string                   `json:"eventId"`
	WatchID             string                   `json:"watchId"`
	DestinationID       string                   `json:"destinationId"`
	DestinationRevision string                   `json:"destinationRevision"`
	Payload             []byte                   `json:"payload"`
	Status              string                   `json:"status"`
	Attempts            int                      `json:"attempts"`
	NextAt              time.Time                `json:"nextAt"`
	CreatedAt           time.Time                `json:"createdAt"`
	LeaseToken          string                   `json:"-"`
	Destination         plan.CompiledDestination `json:"-"`
}

func (t *Tx) Enqueue(i Intent) (int64, error) {
	result, err := t.sql.ExecContext(t.ctx, `INSERT INTO outbox(event_id,watch_id,destination_id,destination_revision,payload,status,next_at,created_at) VALUES(?,?,?,?,?,'pending',?,?)`, i.EventID, i.WatchID, i.DestinationID, i.DestinationRevision, i.Payload, timestamp(i.NextAt), timestamp(i.CreatedAt))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (t *Tx) Intent(id int64) (Intent, error) {
	var i Intent
	var next, created int64
	err := t.sql.QueryRowContext(t.ctx, `SELECT id,event_id,watch_id,destination_id,destination_revision,payload,status,attempts,next_at,created_at,lease_token FROM outbox WHERE id=?`, id).Scan(&i.ID, &i.EventID, &i.WatchID, &i.DestinationID, &i.DestinationRevision, &i.Payload, &i.Status, &i.Attempts, &next, &created, &i.LeaseToken)
	if err != nil {
		return i, missing(err)
	}
	i.NextAt = instant(next)
	i.CreatedAt = instant(created)
	i.Destination, err = t.Destination(i.DestinationID, i.DestinationRevision)
	return i, err
}

// Claim preserves event order per watch/destination. An expired lease becomes
// eligible again with a new token; stale acknowledgments cannot finalize it.
func (s *Store) Claim(ctx context.Context, now time.Time, lease time.Duration) (*Intent, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("lease must be positive")
	}
	var intent *Intent
	err := s.Update(ctx, func(t *Tx) error {
		var id int64
		err := t.sql.QueryRowContext(ctx, `SELECT o.id FROM outbox o WHERE ((o.status='pending' AND o.next_at<=?) OR (o.status='leased' AND o.lease_until<=?)) AND NOT EXISTS (SELECT 1 FROM outbox p WHERE p.watch_id=o.watch_id AND p.destination_id=o.destination_id AND p.id<o.id AND p.status IN ('pending','leased')) ORDER BY o.next_at,o.id LIMIT 1`, timestamp(now), timestamp(now)).Scan(&id)
		if missing(err) == ErrNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		var token [24]byte
		if _, err = rand.Read(token[:]); err != nil {
			return err
		}
		_, err = t.sql.ExecContext(ctx, `UPDATE outbox SET status='leased',lease_token=?,lease_until=?,attempts=attempts+1 WHERE id=?`, hex.EncodeToString(token[:]), timestamp(now.Add(lease)), id)
		if err != nil {
			return err
		}
		i, err := t.Intent(id)
		if err != nil {
			return err
		}
		intent = &i
		return nil
	})
	return intent, err
}
func (s *Store) Finish(ctx context.Context, i Intent, result delivery.Result, nextAt, now time.Time) error {
	status := string(result.Outcome)
	if result.Outcome == delivery.Retryable {
		status = "pending"
	}
	switch status {
	case "pending", "delivered", "permanent", "exhausted", "canceled":
	default:
		return fmt.Errorf("invalid delivery outcome")
	}
	return s.Update(ctx, func(t *Tx) error {
		changed, err := t.sql.ExecContext(ctx, `UPDATE outbox SET status=?,next_at=?,lease_token='',lease_until=0,last_error=? WHERE id=? AND status='leased' AND lease_token=?`, status, timestamp(nextAt), result.Detail, i.ID, i.LeaseToken)
		if err != nil {
			return err
		}
		count, err := changed.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrStale
		}
		_, err = t.sql.ExecContext(ctx, `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) VALUES(?,?,?,?,?)`, i.ID, i.Attempts, timestamp(now), string(result.Outcome), result.Detail)
		return err
	})
}
