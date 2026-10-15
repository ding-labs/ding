package watchrun

import (
	"context"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

// Only the sole execution owner's workers use these hints. Durable snapshots
// reconstruct the cache at startup and during minute maintenance; acceptance and
// lifecycle fencing still read current rows in their own transactions.
type scheduleCache struct {
	mu      sync.Mutex
	state   store.Schedule
	version uint64
	checked time.Time
}

func (c *scheduleCache) load(ctx context.Context, s *store.Store, now time.Time) (store.Schedule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	version := s.ChangeVersion() // Capture before SQL to avoid losing a concurrent commit.
	if !c.checked.IsZero() && version == c.version && now.Before(c.checked.Add(2*time.Minute)) && !now.Before(c.checked) {
		return c.state, nil
	}
	state, err := s.Schedule(ctx)
	if err != nil {
		return state, err
	}
	c.state, c.version, c.checked = state, version, now
	return state, nil
}

func (c *scheduleCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checked = time.Time{}
}

type deliveryCache struct {
	version uint64
	checked time.Time
	empty   bool
}

func (c *deliveryCache) ready(ctx context.Context, s *store.Store, now time.Time, schedule *scheduleCache) (bool, error) {
	version := s.ChangeVersion()
	if c.empty && version == c.version && now.Before(c.checked.Add(2*time.Minute)) && !now.Before(c.checked) {
		return false, nil
	}
	if !c.checked.IsZero() && !c.empty {
		return true, nil
	}
	state, err := schedule.load(ctx, s, now)
	if err != nil {
		return false, err
	}
	c.empty, c.version, c.checked = state.Budget.Pending == 0, version, now
	return !c.empty, nil
}
