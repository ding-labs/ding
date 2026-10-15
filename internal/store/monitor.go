package store

import (
	"context"
	"time"
)

type Monitor struct {
	Running, Pending, Unhealthy    int
	LagSeconds, DeliveryAgeSeconds float64
}

func (s *Store) Monitor(ctx context.Context, now time.Time) (Monitor, error) {
	var m Monitor
	err := s.View(ctx, func(tx *Tx) error {
		var due, oldest int64
		if err := tx.sql.QueryRowContext(ctx, tx.query(qMonitor1)).Scan(&m.Running, &due); err != nil {
			return err
		}
		if err := tx.sql.QueryRowContext(ctx, tx.query(qMonitor2)).Scan(&m.Pending, &oldest); err != nil {
			return err
		}
		if err := tx.sql.QueryRowContext(ctx, tx.query(qMonitor3)).Scan(&m.Unhealthy); err != nil {
			return err
		}
		if due != 0 {
			m.LagSeconds = max(0, now.Sub(instant(due)).Seconds())
		}
		if oldest != 0 {
			m.DeliveryAgeSeconds = max(0, now.Sub(instant(oldest)).Seconds())
		}
		return nil
	})
	return m, err
}
