package logs

import (
	"fmt"
	"regexp"
	"time"
)

type FrameConfig struct {
	Mode       string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Start      string `json:"start,omitempty" yaml:"start,omitempty"`
	Continue   string `json:"continue,omitempty" yaml:"continue,omitempty"`
	MaxLines   int    `json:"maxLines,omitempty" yaml:"maxLines,omitempty"`
	FlushAfter string `json:"flushAfter,omitempty" yaml:"flushAfter,omitempty"`
}

type multiline struct {
	start, continuation *regexp.Regexp
	maxLines, lines     int
	deadline            time.Duration
	buf                 []byte
	invalid             bool
	end                 int64
	timer               *time.Timer
	epoch               uint64
}

func NewConfiguredFramer(c FrameConfig, emit func([]byte, bool)) (*Framer, error) {
	f := NewFramer(emit)
	if c.Mode == "" {
		c.Mode = "line"
	}
	if c.Mode != "multiline" {
		if c.Mode != "line" && c.Mode != "carriage-return" || c.Start != "" || c.Continue != "" || c.MaxLines != 0 || c.FlushAfter != "" {
			return nil, fmt.Errorf("invalid log framing configuration")
		}
		f.carriageReturn = c.Mode == "carriage-return"
		return f, nil
	}
	if c.MaxLines == 0 {
		c.MaxLines = 100
	}
	if c.FlushAfter == "" {
		c.FlushAfter = "1s"
	}
	d, err := time.ParseDuration(c.FlushAfter)
	if err != nil || d < 100*time.Millisecond || d > 5*time.Second || c.MaxLines < 2 || c.MaxLines > 1000 {
		return nil, fmt.Errorf("multiline requires 2–1000 lines and flushAfter between 100ms and 5s")
	}
	if len(c.Start) == 0 || len(c.Start) > 1024 || len(c.Continue) == 0 || len(c.Continue) > 1024 {
		return nil, fmt.Errorf("multiline requires bounded start and continuation patterns")
	}
	start, err := regexp.Compile(c.Start)
	if err != nil {
		return nil, fmt.Errorf("invalid multiline start pattern")
	}
	continuation, err := regexp.Compile(c.Continue)
	if err != nil {
		return nil, fmt.Errorf("invalid multiline continuation pattern")
	}
	f.multi = &multiline{start: start, continuation: continuation, maxLines: c.MaxLines, deadline: d}
	return f, nil
}

func (f *Framer) line(data []byte, invalid bool) {
	m := f.multi
	if m == nil {
		f.deliver(data, invalid, f.read)
		return
	}
	if invalid {
		if m.lines > 0 {
			m.invalid, m.end = true, f.read
		} else {
			f.deliver(nil, true, f.read)
		}
		return
	}
	if m.start.Match(data) {
		f.flush()
		m.lines, m.end = 1, f.read
		m.buf = append(m.buf[:0], data...)
		m.epoch++
		epoch := m.epoch
		m.timer = time.AfterFunc(m.deadline, func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			if !f.closed && m.epoch == epoch {
				f.flush()
			}
		})
		return
	}
	if m.lines > 0 && m.continuation.Match(data) {
		m.lines++
		m.end = f.read
		if m.lines > m.maxLines || len(m.buf)+1+len(data) > MaxRecordBytes {
			m.invalid = true
		}
		if !m.invalid {
			m.buf = append(m.buf, '\n')
			m.buf = append(m.buf, data...)
		}
		return
	}
	f.flush()
	f.deliver(data, false, f.read)
}

func (f *Framer) flush() {
	m := f.multi
	if m == nil || m.lines == 0 {
		return
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	m.epoch++
	f.deliver(m.buf, m.invalid, m.end)
	m.buf, m.lines, m.invalid = m.buf[:0], 0, false
}
