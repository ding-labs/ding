package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Schedule is a coherent snapshot for the single execution owner's worker loop.
// It contains durable deadlines; a cache of it never authorizes a later write.
type Schedule struct {
	Watches []WatchRecord
	Budget  Usage
	TimerAt time.Time
}

func (s *Store) Schedule(ctx context.Context) (Schedule, error) {
	var result Schedule
	err := s.View(ctx, func(tx *Tx) error {
		var err error
		if result.Budget, err = tx.Budget(); err != nil {
			return err
		}
		if result.Watches, err = tx.Watches(); err != nil {
			return err
		}
		var due int64
		err = tx.sql.QueryRowContext(ctx, tx.query(nextTimer)).Scan(&due)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		result.TimerAt = instant(due)
		return err
	})
	return result, err
}

var nextTimer = statement{
	sqlite: `SELECT t.due_at FROM timers t JOIN watches w ON w.id=t.watch_id
 WHERE w.status='running' AND t.generation=w.generation ORDER BY t.due_at LIMIT 1`,
	postgres: `SELECT t.due_at FROM timers t JOIN watches w ON w.workspace_id=t.workspace_id AND w.id=t.watch_id
 WHERE t.workspace_id=current_setting('ding.workspace') AND w.status='running' AND t.generation=w.generation ORDER BY t.due_at LIMIT 1`,
}
