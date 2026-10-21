package watchrun

import (
	"context"
	"time"
)

func earlier(a, b time.Time) time.Time {
	if !b.IsZero() && b.Before(a) {
		return b
	}
	return a
}

// Store notifications are captured before inspecting work. A closed broadcast
// therefore also covers changes made between reading the deadline and sleeping.
func waitUntil(ctx context.Context, changed, completed <-chan struct{}, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-changed:
	case <-completed:
	case <-timer.C:
	}
}
