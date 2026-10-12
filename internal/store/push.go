package store

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrIdempotency = errors.New("idempotency key reused with different request bytes")

// PushReceipt survives evaluator resets. Its namespace belongs to a watch
// lifetime and logical producer, never to a rotating credential or revision.
type PushReceipt struct {
	AcceptanceID string    `json:"acceptanceId"`
	WatchID      string    `json:"watchId"`
	Generation   int64     `json:"generation"`
	Revision     string    `json:"revision"`
	First        int64     `json:"first"`
	Last         int64     `json:"last"`
	Duplicate    bool      `json:"duplicate"`
	AcceptedAt   time.Time `json:"acceptedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type PushUsage struct {
	Requests     int64 `json:"requests"`
	Bytes        int64 `json:"bytes"`
	Observations int64 `json:"observations"`
}

func (t *Tx) PushReceipt(lifetime, producer, key, digest string, now time.Time) (PushReceipt, error) {
	var receipt PushReceipt
	var body []byte
	var stored string
	err := t.sql.QueryRowContext(t.ctx, "SELECT digest,body FROM push_receipts WHERE lifetime=? AND producer=? AND input_key=? AND expires_at>?", lifetime, producer, key, timestamp(now)).Scan(&stored, &body)
	if err != nil {
		return receipt, missing(err)
	}
	if digest != stored {
		return receipt, ErrIdempotency
	}
	err = json.Unmarshal(body, &receipt)
	receipt.Duplicate = true
	return receipt, err
}

func (t *Tx) SavePushReceipt(lifetime, producer, key, digest string, receipt PushReceipt, bytes, observations int) error {
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO push_receipts(lifetime,producer,input_key,digest,body,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(lifetime,producer,input_key) DO UPDATE SET digest=excluded.digest,body=excluded.body,expires_at=excluded.expires_at`, lifetime, producer, key, digest, body, timestamp(receipt.ExpiresAt))
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, "INSERT INTO push_last(watch,lifetime,body) VALUES(?,?,?) ON CONFLICT(watch) DO UPDATE SET lifetime=excluded.lifetime,body=excluded.body", receipt.WatchID, lifetime, body)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, "UPDATE push_usage SET requests=requests+1,bytes=bytes+?,observations=observations+? WHERE singleton=1", bytes, observations)
	return err
}

func (t *Tx) PushReceiptCount(now time.Time) (int, error) {
	var n int
	err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM push_receipts WHERE expires_at>?", timestamp(now)).Scan(&n)
	return n, err
}

func (t *Tx) PushUsage() (PushUsage, error) {
	var u PushUsage
	err := t.sql.QueryRowContext(t.ctx, "SELECT requests,bytes,observations FROM push_usage WHERE singleton=1").Scan(&u.Requests, &u.Bytes, &u.Observations)
	return u, err
}

func (t *Tx) LastPush(lifetime, watch string) (PushReceipt, error) {
	var out PushReceipt
	var body []byte
	if err := t.sql.QueryRowContext(t.ctx, "SELECT body FROM push_last WHERE lifetime=? AND watch=?", lifetime, watch).Scan(&body); err != nil {
		return out, missing(err)
	}
	err := json.Unmarshal(body, &out)
	return out, err
}
