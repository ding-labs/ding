package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/watchrun"
)

// Ingestion follows the API handler's record lookup for every request. A watch
// generation is a fence for one acquisition, not a cache valid for the whole run.
func ingestCurrent(ctx context.Context, app *watchrun.App, body []byte, id string) error {
	record, err := app.Record(ctx, "push")
	if err != nil {
		return err
	}
	_, err = app.IngestReserved(ctx, record, body, id)
	return err
}

// Linux VM suspension can stop its monotonic clock while wall time advances.
// Process suspension can instead advance both clocks but interrupt the load.
// Neither is a continuous capacity fixture; never make up missed observations.
func checkContinuity(wall, monotonic, gap time.Duration) error {
	drift := wall - monotonic
	if drift > 2*time.Second || drift < -2*time.Second {
		return fmt.Errorf("clock discontinuity: wall and monotonic elapsed differ by %s; restart the continuous fixture", drift)
	}
	if gap > 10*time.Second {
		return fmt.Errorf("load interrupted for %s; restart the continuous fixture", gap)
	}
	return nil
}
