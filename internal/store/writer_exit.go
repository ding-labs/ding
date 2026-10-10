package store

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// WaitWriterStopped waits for the durable writer lock without opening SQLite.
// It cannot promise that a different supervisor will not start another writer.
func WaitWriterStopped(ctx context.Context, dir string) error {
	for {
		lock, err := lockDirectory(filepath.Join(dir, "writer.lock"))
		if err == nil {
			return unlockDirectory(lock)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("writer did not release its state lock; inspect the daemon before retrying: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
