package watchrun

import (
	"context"
	"io"

	"github.com/ding-labs/ding/internal/delivery"
)

// Generic io.Writer has no cancellation contract. One permit bounds writes to
// one outstanding goroutine even if stdout is a blocked pipe. Delivery workers
// can stop; a late write may duplicate receipt, just like an ambiguous HTTP ack.
// Embedders should supply a writer they can close to release a blocked write.
func (a *App) console(ctx context.Context, payload []byte) delivery.Result {
	a.outputMu.Lock()
	if a.outputPermit == nil {
		a.outputPermit = make(chan struct{}, 1)
	}
	permit := a.outputPermit
	output := a.Output
	a.outputMu.Unlock()
	select {
	case permit <- struct{}{}:
	case <-ctx.Done():
		return delivery.Result{Outcome: delivery.Retryable, Detail: "console_interrupted"}
	}
	done := make(chan error, 1)
	go func() {
		defer func() { <-permit }()
		body := string(payload) + "\n"
		n, err := io.WriteString(output, body)
		if err == nil && n != len(body) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return delivery.Result{Outcome: delivery.Retryable, Detail: "console_write_failed"}
		}
		return delivery.Result{Outcome: delivery.Delivered}
	case <-ctx.Done():
		return delivery.Result{Outcome: delivery.Retryable, Detail: "console_interrupted"}
	}
}
