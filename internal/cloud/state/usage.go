package state

import (
	"context"
	"fmt"
	"time"
)

const MonthlyChecks = 30000
const MonthlyDeliveries = 1000
const MonthlyBytes = 128 << 20
const RequestReservation = 192 << 10

type Usage struct {
	Period     string `json:"period"`
	Checks     int    `json:"checks"`
	Deliveries int    `json:"deliveries"`
	Bytes      int64  `json:"bytes"`
}
type Reservation struct{ ID, Account, Period string }

func (d *DB) Reserve(ctx context.Context, account string, delivery bool, now time.Time) (Reservation, error) {
	r := Reservation{ID: ID(), Account: account, Period: now.UTC().Format("2006-01")}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	// Crashed requests retain their conservative byte charge, but not an
	// indefinitely growing reservation row. No external action is retried here.
	if _, err := tx.ExecContext(ctx, "DELETE FROM reservations WHERE account=? AND created_at<?", account, now.Add(-time.Hour).Unix()); err != nil {
		return r, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM reservations WHERE account=?", account).Scan(&active); err != nil {
		return r, err
	}
	if active >= 4 {
		return r, fmt.Errorf("workspace outbound concurrency exhausted")
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO usage(account,period) VALUES(?,?)", account, r.Period); err != nil {
		return r, err
	}
	checks, deliveries := 1, 0
	if delivery {
		checks, deliveries = 0, 1
	}
	result, err := tx.ExecContext(ctx, `UPDATE usage SET checks=checks+?,deliveries=deliveries+?,bytes=bytes+?
WHERE account=? AND period=? AND checks+?<=? AND deliveries+?<=? AND bytes+?<=?`, checks, deliveries, RequestReservation, account, r.Period, checks, MonthlyChecks, deliveries, MonthlyDeliveries, RequestReservation, MonthlyBytes)
	if err != nil {
		return r, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return r, err
	}
	if changed != 1 {
		return r, fmt.Errorf("workspace monthly outbound budget exhausted; inspect cloud usage")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO reservations(id,account,period,created_at) VALUES(?,?,?,?)", r.ID, account, r.Period, now.Unix()); err != nil {
		return r, err
	}
	return r, tx.Commit()
}

// Finish is durable and idempotent. Charges include request/response bodies and
// a bounded header allowance; encrypted wire overhead is tracked operationally.
func (d *DB) Finish(ctx context.Context, r Reservation, used int64) error {
	if used < 0 || used > RequestReservation {
		return fmt.Errorf("invalid network charge")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM reservations WHERE id=? AND account=? AND period=?", r.ID, r.Account, r.Period)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE usage SET bytes=bytes-? WHERE account=? AND period=?", RequestReservation-used, r.Account, r.Period); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) Usage(ctx context.Context, account string, now time.Time) (Usage, error) {
	u := Usage{Period: now.UTC().Format("2006-01")}
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE((SELECT checks FROM usage WHERE account=? AND period=?),0),COALESCE((SELECT deliveries FROM usage WHERE account=? AND period=?),0),COALESCE((SELECT bytes FROM usage WHERE account=? AND period=?),0)`, account, u.Period, account, u.Period, account, u.Period).Scan(&u.Checks, &u.Deliveries, &u.Bytes)
	return u, err
}
