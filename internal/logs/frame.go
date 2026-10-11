package logs

import "bytes"

const MaxRecordBytes = 64 << 10

// Framer accepts arbitrary write boundaries. It retains at most 64 KiB, and
// reports one invalid record after draining an oversized line to its boundary.
// emit must consume/copy the supplied bytes before returning.
type Framer struct {
	buf            []byte
	oversize       bool
	emit           func([]byte, bool)
	read, complete int64
}

func NewFramer(emit func([]byte, bool)) *Framer { return &Framer{emit: emit} }

func (f *Framer) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
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
		f.complete = f.read
		f.emit(f.buf, f.oversize)
		f.buf, f.oversize = f.buf[:0], false
		p = p[end+1:]
	}
	return n, nil
}

// Completed is the byte count through the last complete newline boundary.
func (f *Framer) Completed() int64 { return f.complete }

// Close emits a final unterminated record, including an oversized one.
func (f *Framer) Close() error {
	if len(f.buf) > 0 || f.oversize {
		f.emit(f.buf, f.oversize)
	}
	f.buf, f.oversize = nil, false
	return nil
}
