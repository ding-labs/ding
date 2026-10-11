package logs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

var ErrFileRead = errors.New("log_file_unavailable")

// FilePosition describes the last completed boundary, not a promise of replay.
// The fingerprint detects overwrites near that boundary, without retaining text.
type FilePosition struct {
	Identity    string `json:"identity"`
	Offset      int64  `json:"offset"`
	LagBytes    int64  `json:"lagBytes"`
	SkippedFrom int64  `json:"skippedFrom,omitempty"`
	SkippedTo   int64  `json:"skippedTo,omitempty"`
}

type FileFollower struct {
	Path      string
	Collector *Collector
	mu        sync.Mutex
	position  FilePosition
}

func (f *FileFollower) Position() FilePosition {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.position
}

// Run tails a regular file from its current end. At rotation it drains the old
// descriptor, then skips to the replacement's end and records a gap. This keeps
// old backlog out of live windows and bounds descriptor ownership to one file.
func (f *FileFollower) Run(ctx context.Context) error {
	var file *os.File
	defer func() {
		if file != nil {
			file.Close()
		}
	}()
	var info os.FileInfo
	var frame *Framer
	var base, offset int64
	var fingerprint []byte
	open := func() error {
		var err error
		file, err = openLogFile(f.Path)
		if err != nil {
			return ErrFileRead
		}
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return ErrFileRead
		}
		offset, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return ErrFileRead
		}
		base, frame = offset, f.Collector.Stream()
		fingerprint = fileTail(file, offset)
		f.mu.Lock()
		f.position = FilePosition{Identity: fileIdentity(info), Offset: offset, SkippedTo: offset}
		f.mu.Unlock()
		return nil
	}
	if err := open(); err != nil {
		return err
	}
	buffer := make([]byte, 32<<10)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Check both size and a bounded suffix: copytruncate can regrow past
		// the old offset before the next read.
		current, err := file.Stat()
		if err != nil {
			return ErrFileRead
		}
		if current.Size() < offset || current.Size()-offset > 4<<20 || !bytes.Equal(fingerprint, fileTail(file, offset)) {
			f.Collector.Gap()
			old := base + frame.Completed()
			offset, err = file.Seek(0, io.SeekEnd)
			if err != nil {
				return ErrFileRead
			}
			base, frame = offset, f.Collector.Stream()
			fingerprint = fileTail(file, offset)
			f.mu.Lock()
			f.position.SkippedFrom, f.position.SkippedTo = old, offset
			f.mu.Unlock()
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			frame.Write(buffer[:n])
			offset += int64(n)
			fingerprint = fileTail(file, offset)
		}
		f.mu.Lock()
		f.position.Offset = base + frame.Completed()
		f.position.LagBytes = max(0, current.Size()-offset)
		f.mu.Unlock()
		if readErr != nil && readErr != io.EOF {
			return ErrFileRead
		}
		if n > 0 {
			continue
		}
		pathInfo, err := os.Stat(f.Path)
		if err != nil {
			return ErrFileRead
		}
		if !os.SameFile(info, pathInfo) {
			f.Collector.Gap() // Includes an abandoned partial record, if any.
			file.Close()
			if err := open(); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func fileTail(file *os.File, offset int64) []byte {
	n := min(offset, 64)
	b := make([]byte, n)
	read, _ := file.ReadAt(b, offset-n)
	return b[:read]
}
