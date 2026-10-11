package logs

import (
	"sync"
	"time"
)

type record struct {
	data     []byte
	invalid  bool
	snapshot chan Summary
}

// Collector separates output draining from analysis. Its 64-record queue uses
// at most 4 MiB of record bytes; overload invalidates the window, not the child.
type Collector struct {
	window *Window
	parser *Parser
	queue  chan record
	done   chan struct{}
	once   sync.Once
}

func NewCollector(parser ParserConfig, window WindowConfig) (*Collector, error) {
	p, err := NewParser(parser)
	if err != nil {
		return nil, err
	}
	w, err := NewWindow(window, time.Now())
	if err != nil {
		return nil, err
	}
	c := &Collector{parser: p, window: w, queue: make(chan record, 64), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for r := range c.queue {
			if r.snapshot != nil {
				r.snapshot <- w.Snapshot(time.Now())
				continue
			}
			classification := Classification{Invalid: true}
			if !r.invalid {
				classification = p.Classify(r.data)
			}
			// Collection time is when the bounded analysis queue accepts a record
			// for classification. No application timestamp enters the window.
			w.Add(time.Now(), classification)
		}
	}()
	return c, nil
}

// Stream returns a framer for a single output channel. Close each stream before
// closing the collector. Callers preserve original bytes separately.
func (c *Collector) Stream() *Framer {
	return NewFramer(func(data []byte, invalid bool) {
		select {
		case c.queue <- record{data: append([]byte(nil), data...), invalid: invalid}:
		default:
			c.window.Gap(time.Now())
		}
	})
}

func (c *Collector) Snapshot() Summary {
	result := make(chan Summary, 1)
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-c.done:
		return c.window.Snapshot(time.Now())
	default:
	}
	// Snapshot is a barrier behind already queued records.
	select {
	case c.queue <- record{snapshot: result}:
		select {
		case s := <-result:
			return s
		case <-timer.C:
		}
	case <-timer.C:
	}
	c.window.Gap(time.Now())
	return c.window.Snapshot(time.Now())
}

func (c *Collector) Gap() { c.window.Gap(time.Now()) }

// Close is called after all stream writers and snapshot publishers have stopped.
func (c *Collector) Close() {
	c.once.Do(func() { close(c.queue) })
	<-c.done
}
