package store

import (
	"context"
	"fmt"
	"time"
)

// QuarantineRestore prevents a stale backup from resurrecting alert producers,
// external deliveries or model credentials. History is preserved for inspection.
func (s *Store) QuarantineRestore(ctx context.Context) error {
	health, err := s.Health(ctx)
	if err != nil {
		return err
	}
	if health.Integrity != "ok" {
		return fmt.Errorf("restored workspace failed integrity check")
	}
	return s.Update(ctx, func(tx *Tx) error {
		records, err := tx.Watches()
		if err != nil {
			return err
		}
		for _, r := range records {
			if r.Status == "deleted" {
				continue
			}
			id := r.Plan.Definition.Metadata.ID
			r.Status = "paused"
			r.Generation++
			r.LastError = "restored backup: verify execution ownership and inspect canceled deliveries before resuming"
			if err := tx.SaveWatch(r, time.Now()); err != nil {
				return err
			}
			if err := tx.DeleteTimers(id); err != nil {
				return err
			}
			if err := tx.CancelDeliveries(id, time.Now()); err != nil {
				return err
			}
		}
		grants, err := tx.IntegrationGrants()
		if err != nil {
			return err
		}
		for _, g := range grants {
			if err := tx.RevokeIntegrationGrant(g.ID); err != nil {
				return err
			}
		}
		moves, err := tx.Handoffs()
		if err != nil {
			return err
		}
		for _, h := range moves {
			h.Phase = "restore-required"
			if err := tx.SaveHandoff(h); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) ReleaseRestoredHolds(ctx context.Context) error {
	return s.Update(ctx, func(tx *Tx) error {
		moves, err := tx.Handoffs()
		if err != nil {
			return err
		}
		for _, h := range moves {
			if h.Phase == "restore-required" {
				h.Held = false
				h.Phase = "restored"
				if err := tx.SaveHandoff(h); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
