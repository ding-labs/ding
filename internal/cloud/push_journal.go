package cloud

import (
	"bufio"
	"encoding/json"
	"sync"
)

// plannedPushJournal is used only after all possible request keys are durable.
// A bounded queue isolates response recording from individual fsync latency.
// Backpressure is intentional: an exhausted queue must slow the generator and
// invalidate its timing, never discard evidence or allocate without a bound.
func plannedPushJournal(writer syncedWriter, capacity int) (func(pushLedgerEntry) error, func() error) {
	queue := make(chan pushLedgerEntry, capacity)
	done := make(chan struct{})
	var mu sync.Mutex
	var failure error
	go func() {
		defer close(done)
		buffer := bufio.NewWriterSize(writer, 64<<10)
		encoder := json.NewEncoder(buffer)
		for entry := range queue {
			if failure != nil {
				continue
			}
			err := encoder.Encode(entry)
			if err == nil && len(queue) == 0 {
				err = buffer.Flush()
				if err == nil {
					err = writer.Sync()
				}
			}
			if err != nil {
				mu.Lock()
				failure = err
				mu.Unlock()
			}
		}
	}()
	return func(entry pushLedgerEntry) error {
			queue <- entry
			mu.Lock()
			defer mu.Unlock()
			return failure
		}, func() error {
			close(queue)
			<-done
			return failure
		}
}
