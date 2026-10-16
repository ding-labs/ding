package store

import (
	"database/sql"
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
	err := t.sql.QueryRowContext(t.ctx, t.query(qPushReceipt), lifetime, producer, key, timestamp(now)).Scan(&stored, &body)
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
	if t.postgres {
		// Persist the receipt, latest receipt and durable usage together without
		// three network round trips. The caller still commits the whole ingest.
		_, err = t.exec(qSavePushCompletion, lifetime, producer, key, digest, body,
			timestamp(receipt.ExpiresAt), receipt.WatchID, bytes, observations)
		return err
	}
	_, err = t.exec(qSavePushReceipt, lifetime, producer, key, digest, body, timestamp(receipt.ExpiresAt))
	if err != nil {
		return err
	}
	_, err = t.exec(qSaveLastPush, receipt.WatchID, lifetime, body)
	if err != nil {
		return err
	}
	_, err = t.exec(qAddPushUsage, bytes, observations)
	return err
}

func (t *Tx) PushReceiptCount(now time.Time) (int, error) {
	var total, expired, checked, earliest int64
	err := t.sql.QueryRowContext(t.ctx, t.query(qPushReceiptStats)).Scan(&total, &expired, &checked, &earliest)
	if err == sql.ErrNoRows {
		return t.rebuildPushReceiptCount(now)
	}
	if err != nil {
		return 0, err
	}
	at := timestamp(now)
	if total == 0 || at < earliest {
		return int(total), nil
	}
	n := expired
	if at < checked {
		// A backward clock jump may make retained receipts active again.
		err = t.sql.QueryRowContext(t.ctx, t.query(qExpiredPushCount), at).Scan(&n)
	} else if at > checked {
		var due int64
		err = t.sql.QueryRowContext(t.ctx, t.query(qNewExpiredPushCount), checked, at).Scan(&due)
		n += due
	}
	if err != nil {
		return 0, err
	}
	// Readers compute exact counts without mutating their snapshot. Admission
	// advances the watermark only when expiry changes the cached population.
	if t.writable && (n != expired || at < checked) {
		_, err = t.exec(qAdvancePushExpiry, n, at)
	}
	return int(total - n), err
}

func (t *Tx) rebuildPushReceiptCount(now time.Time) (int, error) {
	var total, expired, earliest int64
	at := timestamp(now)
	err := t.sql.QueryRowContext(t.ctx, t.query(qRebuildPushCount), at).Scan(&total, &expired, &earliest)
	if err == nil && t.writable {
		_, err = t.exec(qSavePushCount, total, expired, at, earliest)
	}
	return int(total - expired), err
}

func (t *Tx) PushUsage() (PushUsage, error) {
	var u PushUsage
	err := t.sql.QueryRowContext(t.ctx, t.query(qPushUsage)).Scan(&u.Requests, &u.Bytes, &u.Observations)
	return u, err
}

func (t *Tx) LastPush(lifetime, watch string) (PushReceipt, error) {
	var out PushReceipt
	var body []byte
	if err := t.sql.QueryRowContext(t.ctx, t.query(qLastPush), lifetime, watch).Scan(&body); err != nil {
		return out, missing(err)
	}
	err := json.Unmarshal(body, &out)
	return out, err
}
