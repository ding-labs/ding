package watchrun

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
	"time"
)

func (a *App) nextPoll(now time.Time, interval time.Duration, id string) time.Time {
	next := now.Add(interval)
	if a.SchedulingJitter <= 0 {
		return next
	}
	// Positive bounded jitter never polls faster than the reviewed cadence.
	bound := min(a.SchedulingJitter, 5*time.Second)
	digest := sha256.Sum256([]byte(id + "/" + strconv.FormatInt(now.UnixNano(), 10)))
	return next.Add(time.Duration(binary.LittleEndian.Uint64(digest[:8]) % uint64(bound)))
}
