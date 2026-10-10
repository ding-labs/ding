package cloud

import (
	"context"
	"fmt"
	"io"
	"time"
)

func (p *Pool) Maintain(ctx context.Context, out io.Writer) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		lastGC := time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			task, stop := context.WithTimeout(ctx, 30*time.Second)
			if err := p.FinishPendingDeletes(task); err != nil {
				fmt.Fprintln(out, "Pending workspace deletion needs operator attention.")
			}
			if time.Since(lastGC) >= time.Hour {
				if err := p.db.Collect(task, time.Now()); err != nil {
					fmt.Fprintln(out, "Control metadata retention needs operator attention.")
				} else {
					lastGC = time.Now()
				}
			}
			stop()
		}
	}()
	return func() { cancel(); <-done }
}
