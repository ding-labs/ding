package logs

import (
	"sync"
	"time"
)

type record struct {
	data     []byte
	invalid  bool
	snapshot chan Summary
	at       time.Time
	channel  string
	epoch    uint64
}

// Collector separates output draining from analysis. Its queue bounds both
// records (4096) and copied bytes (4 MiB); overload invalidates the window.
type Collector struct {
	window      *Window
	parser      *Parser
	queue       chan record
	done        chan struct{}
	once        sync.Once
	mu          sync.Mutex
	epoch       uint64
	framing     FrameConfig
	queuedBytes int
	statistics  Statistics
}

func NewCollector(parser ParserConfig, window WindowConfig, frames ...FrameConfig) (*Collector, error) {
	framing := FrameConfig{}
	if len(frames) > 0 {
		framing = frames[0]
	}
	if _, err := NewConfiguredFramer(framing, func([]byte, bool) {}); err != nil {
		return nil, err
	}
	p, err := NewParser(parser)
	if err != nil {
		return nil, err
	}
	w, err := NewWindow(window, time.Now())
	if err != nil {
		return nil, err
	}
	c := &Collector{parser: p, window: w, queue: make(chan record, 4096), done: make(chan struct{}), framing: framing}
	go func() {
		defer close(c.done)
		for r := range c.queue {
			if r.snapshot != nil {
				r.snapshot <- w.Snapshot(r.at)
				continue
			}
			classification := Classification{Invalid: true}
			if !r.invalid {
				classification = p.Classify(r.data)
			}
			c.mu.Lock()
			c.queuedBytes -= len(r.data)
			if r.epoch == c.epoch {
				w.AddFrom(r.at, classification, r.channel)
				if !classification.Blank {
					c.statistics.Records++
					if classification.Level == Warning {
						c.statistics.Warnings++
					}
					if classification.Level == Error || classification.Level == Fatal {
						c.statistics.Errors++
					}
					if classification.Level == Unclassified {
						c.statistics.Unclassified++
					}
					if classification.Invalid {
						c.statistics.Invalid++
					}
				}
			}
			c.mu.Unlock()
		}
	}()
	return c, nil
}

// Stream returns a framer for a single output channel. Close each stream before
// closing the collector. Callers preserve original bytes separately.
func (c *Collector) Stream(channels ...string) *Framer {
	channel := ""
	if len(channels) > 0 {
		channel = channels[0]
	}
	f, _ := NewConfiguredFramer(c.framing, func(data []byte, invalid bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		at := time.Now() // Completion enters the queue atomically with its timestamp.
		if c.queuedBytes+len(data) > 4<<20 {
			c.statistics.Dropped++
			c.window.Drop(at)
			return
		}
		select {
		case c.queue <- record{data: append([]byte(nil), data...), invalid: invalid, at: at, channel: channel, epoch: c.epoch}:
			c.queuedBytes += len(data)
		default:
			c.statistics.Dropped++
			c.window.Drop(at)
		}
	})
	return f
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
	// Freeze the boundary when the barrier enters the queue. Classification may
	// finish in a later second; records queued behind it must remain outside it.
	c.mu.Lock()
	select {
	case c.queue <- record{snapshot: result, at: time.Now()}:
		c.mu.Unlock()
	default:
		c.mu.Unlock()
		c.Gap()
		return c.window.Snapshot(time.Now())
	}
	select {
	case s := <-result:
		return s
	case <-timer.C:
	}
	c.window.Gap(time.Now())
	return c.window.Snapshot(time.Now())
}

func (c *Collector) Gap() { c.window.Gap(time.Now()) }

func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	c.statistics = Statistics{}
	c.window.Reset(time.Now())
}

// Close is called after all stream writers and snapshot publishers have stopped.
func (c *Collector) Close() {
	c.once.Do(func() { close(c.queue) })
	<-c.done
}
