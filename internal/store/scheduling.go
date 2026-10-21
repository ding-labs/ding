package store

import (
	"context"
	"database/sql"
	"time"
)

// NextDelivery considers each active stream's oldest live intent, including
// lease expiry and destination backoff. Blocked successors must not contribute
// old deadlines, which would make sleeping workers spin.
func (s *Store) NextDelivery(ctx context.Context) (time.Time, error) {
	var next sql.NullInt64
	err := s.View(ctx, func(tx *Tx) error {
		return tx.sql.QueryRowContext(ctx, tx.query(nextDelivery)).Scan(&next)
	})
	if !next.Valid {
		return time.Time{}, err
	}
	return instant(next.Int64), err
}

var nextDelivery = statement{
	sqlite: `SELECT MIN(MAX(
 CASE WHEN o.status='leased' THEN o.lease_until ELSE o.next_at END,
 COALESCE((SELECT CAST(m.value AS INTEGER) FROM metadata m WHERE m.key='delivery_backoff:'||o.destination_id||':'||o.destination_revision),0),
 COALESCE((SELECT MAX(p.lease_until) FROM outbox p WHERE p.watch_id=o.watch_id AND p.destination_id=o.destination_id AND p.destination_revision=o.destination_revision AND p.status='leased'),0)))
 FROM delivery_streams s INDEXED BY delivery_stream_active CROSS JOIN outbox o
 WHERE s.active=1 AND o.id=(SELECT p.id FROM outbox p INDEXED BY outbox_live_order WHERE p.watch_id=s.watch_id AND p.destination_id=s.destination_id AND p.status IN ('pending','leased') ORDER BY p.id LIMIT 1)`,
	postgres: `SELECT MIN(GREATEST(
 CASE WHEN o.status='leased' THEN o.lease_until ELSE o.next_at END,
 COALESCE((SELECT CASE WHEN m.key='delivery_backoff:'||o.destination_id||':'||o.destination_revision THEN CAST(m.value AS BIGINT) ELSE 0 END FROM metadata m WHERE m.workspace_id=o.workspace_id AND m.key='delivery_backoff:'||o.destination_id||':'||o.destination_revision),0),
 COALESCE((SELECT MAX(p.lease_until) FROM outbox p WHERE p.workspace_id=o.workspace_id AND p.watch_id=o.watch_id AND p.destination_id=o.destination_id AND p.destination_revision=o.destination_revision AND p.status='leased'),0)))
 FROM delivery_streams s CROSS JOIN LATERAL
 (SELECT * FROM outbox p WHERE p.workspace_id=s.workspace_id AND p.watch_id=s.watch_id AND p.destination_id=s.destination_id AND p.status IN ('pending','leased') ORDER BY p.id LIMIT 1) o
 WHERE s.workspace_id=current_setting('ding.workspace') AND s.active`,
}
