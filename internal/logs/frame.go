package logs

import (
	"bytes"
	"io"
	"sync"
)

const MaxRecordBytes = 64 << 10

// Framer accepts arbitrary write boundaries. It retains at most 64 KiB, and
// reports one invalid record after draining an oversized line to its boundary.
// emit must consume/copy the supplied bytes before returning.
type Framer struct {
	buf            []byte
	oversize       bool
	emit           func([]byte, bool)
	read, complete int64
	mu             sync.Mutex
	closed         bool
	carriageReturn bool
	multi          *multiline
}

func NewFramer(emit func([]byte, bool)) *Framer { return &Framer{emit: emit} }

func (f *Framer) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		if f.carriageReturn {
			if cr := bytes.IndexByte(p, '\r'); cr >= 0 && (end < 0 || cr < end) {
				end = cr
			}
		}
		part := p
		if end >= 0 {
			part = p[:end]
		}
		if !f.oversize {
			if len(f.buf)+len(part) > MaxRecordBytes {
				f.oversize = true
				f.buf = f.buf[:0]
			} else {
				f.buf = append(f.buf, part...)
			}
		}
		if end < 0 {
			f.read += int64(len(p))
			break
		}
		f.read += int64(end + 1)
		f.line(f.buf, f.oversize)
		f.buf, f.oversize = f.buf[:0], false
		p = p[end+1:]
	}
	return n, nil
}

// Completed is the byte count through the last complete newline boundary.
func (f *Framer) Completed() int64 { f.mu.Lock(); defer f.mu.Unlock(); return f.complete }

func (f *Framer) deliver(data []byte, invalid bool, end int64) {
	f.complete = end
	f.emit(data, invalid)
}

// Close emits a final unterminated record, including an oversized one.
func (f *Framer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	if len(f.buf) > 0 || f.oversize {
		f.line(f.buf, f.oversize)
	}
	f.flush()
	f.buf, f.oversize = nil, false
	f.closed = true
	return nil
}

// Discard abandons partial framing on a transport gap. It also stops the
// multiline timer before its owner can close the collector.
func (f *Framer) Discard() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed, f.buf = true, nil
	if f.multi != nil {
		if f.multi.timer != nil {
			f.multi.timer.Stop()
		}
		f.multi.buf = nil
	}
}
