package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

type syncedWriter interface {
	io.Writer
	Sync() error
}
type ledgerRequest struct {
	entry pushLedgerEntry
	ack   chan error
}

// Each caller is released only after its complete group has reached Sync.
// Batching changes sync frequency, never the offered-before-submit ordering.
func groupedPushLedger(writer syncedWriter, batch int) (func(pushLedgerEntry) error, func()) {
	requests := make(chan ledgerRequest, batch)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var failure error
		for first := range requests {
			group := []ledgerRequest{first}
			closed := false
			if batch > 1 {
				timer := time.NewTimer(2 * time.Millisecond)
			collect:
				for len(group) < batch {
					select {
					case next, ok := <-requests:
						if !ok {
							closed = true
							break collect
						}
						group = append(group, next)
					case <-timer.C:
						break collect
					}
				}
				timer.Stop()
			}
			if failure == nil {
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				for _, r := range group {
					if err := enc.Encode(r.entry); err != nil {
						failure = err
						break
					}
				}
				if failure == nil {
					var n int
					n, failure = writer.Write(buf.Bytes())
					if failure == nil && n != buf.Len() {
						failure = io.ErrShortWrite
					}
				}
				if failure == nil {
					failure = writer.Sync()
				}
			}
			for _, r := range group {
				r.ack <- failure
			}
			if closed {
				return
			}
		}
	}()
	return func(e pushLedgerEntry) error {
		ack := make(chan error, 1)
		requests <- ledgerRequest{e, ack}
		return <-ack
	}, func() { close(requests); <-done }
}
