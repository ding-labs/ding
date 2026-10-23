package state

import (
	"context"
	"fmt"
	"time"
)

const MonthlyChecks = 30000
const MonthlyDeliveries = 1000
const MonthlyBytes = 128 << 20

// Cover a 1 MiB response, its overflow probe, and the transport's combined
// request/header allowance. A crashed request retains this conservative charge.
const RequestReservation = (1 << 20) + 1 + (96 << 10)

type Usage struct {
	Period     string `json:"period"`
	Checks     int    `json:"checks"`
	Deliveries int    `json:"deliveries"`
	Bytes      int64  `json:"bytes"`
}
type Reservation struct{ ID, Workspace, Period string }
type OutboundBudget struct {
	Checks, Deliveries int
	Bytes              int64
}

func (d *DB) Reserve(ctx context.Context, workspace string, delivery bool, now time.Time) (Reservation, error) {
	return d.ReserveBudget(ctx, workspace, delivery, now, OutboundBudget{MonthlyChecks, MonthlyDeliveries, MonthlyBytes})
}
func (d *DB) ReserveBudget(ctx context.Context, workspace string, delivery bool, now time.Time, budget OutboundBudget) (Reservation, error) {
	r := Reservation{ID: ID(), Workspace: workspace, Period: now.UTC().Format("2006-01")}
	tx, err := d.begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	// Crashed requests retain their conservative byte charge, but not an
	// indefinitely growing reservation row. No external action is retried here.
	if _, err := tx.ExecContext(ctx, tx.query(qReserveBudget1), workspace, now.Add(-time.Hour).Unix()); err != nil {
		return r, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, tx.query(qReserveBudget2), workspace).Scan(&active); err != nil {
		return r, err
	}
	if active >= 4 {
		return r, fmt.Errorf("workspace outbound concurrency exhausted")
	}
	if _, err := tx.ExecContext(ctx, tx.query(qReserveBudget3), workspace, r.Period); err != nil {
		return r, err
	}
	checks, deliveries := 1, 0
	if delivery {
		checks, deliveries = 0, 1
	}
	result, err := tx.ExecContext(ctx, tx.query(qReserveBudget4), checks, deliveries, RequestReservation, workspace, r.Period, budget.Checks, checks, budget.Checks, budget.Deliveries, deliveries, budget.Deliveries, budget.Bytes, RequestReservation, budget.Bytes)
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
	if _, err := tx.ExecContext(ctx, tx.query(qReserveBudget5), r.ID, workspace, r.Period, now.Unix()); err != nil {
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
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.query(qFinish1), r.ID, r.Workspace, r.Period)
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
	if _, err := tx.ExecContext(ctx, tx.query(qFinish2), RequestReservation-used, r.Workspace, r.Period); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) Usage(ctx context.Context, workspace string, now time.Time) (Usage, error) {
	u := Usage{Period: now.UTC().Format("2006-01")}
	err := d.sql.QueryRowContext(ctx, d.query(qUsage1), workspace, u.Period, workspace, u.Period, workspace, u.Period).Scan(&u.Checks, &u.Deliveries, &u.Bytes)
	return u, err
}
