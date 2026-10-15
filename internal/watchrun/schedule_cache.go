package watchrun

import (
	"context"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

// Only the sole execution owner's workers use these hints. Durable snapshots
// reconstruct the cache at startup and at least once a minute; acceptance and
// lifecycle fencing still read current rows in their own transactions.
type scheduleCache struct {
	state   store.Schedule
	version uint64
	checked time.Time
}

func (c *scheduleCache) load(ctx context.Context, s *store.Store, now time.Time) (store.Schedule, error) {
	version := s.ChangeVersion() // Capture before SQL to avoid losing a concurrent commit.
	if !c.checked.IsZero() && version == c.version && now.Before(c.checked.Add(time.Minute)) && !now.Before(c.checked) {
		return c.state, nil
	}
	state, err := s.Schedule(ctx)
	if err != nil {
		return state, err
	}
	c.state, c.version, c.checked = state, version, now
	return state, nil
}

type deliveryCache struct {
	version uint64
	checked time.Time
	empty   bool
}

func (c *deliveryCache) ready(ctx context.Context, s *store.Store, now time.Time) (bool, error) {
	version := s.ChangeVersion()
	if c.empty && version == c.version && now.Before(c.checked.Add(time.Minute)) && !now.Before(c.checked) {
		return false, nil
	}
	if !c.checked.IsZero() && !c.empty {
		return true, nil
	}
	var pending int
	err := s.View(ctx, func(tx *store.Tx) error { var err error; pending, err = tx.Pending(); return err })
	if err != nil {
		return false, err
	}
	c.empty, c.version, c.checked = pending == 0, version, now
	return !c.empty, nil
}
