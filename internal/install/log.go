package install

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// Log keeps a bounded current file and two previous files for background jobs.
// It must have a single process owner (the daemon's state lock provides that).
type Log struct {
	mu    sync.Mutex
	path  string
	limit int64
	file  *os.File
	size  int64
}

func OpenLog(path string, limit int64) (*Log, error) {
	if limit < 1024 {
		return nil, fmt.Errorf("log limit must be at least 1024 bytes")
	}
	l := &Log{path: path, limit: limit}
	return l, l.open()
}
func (l *Log) open() error {
	info, err := os.Lstat(l.path)
	if errors.Is(err, os.ErrNotExist) {
		f, e := mcpconfig.CreatePrivate(l.path)
		if e != nil {
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
	} else if err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("log must be a regular file")
	}
	if err := mcpconfig.CheckPrivate(l.path); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err = f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.file = f
	l.size = info.Size()
	return nil
}
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return 0, os.ErrClosed
	}
	if l.size+int64(len(p)) > l.limit {
		if err := l.file.Close(); err != nil {
			return 0, err
		}
		l.file = nil
		if err := os.Remove(l.path + ".2"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		if err := os.Rename(l.path+".1", l.path+".2"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return 0, err
		}
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	original := len(p)
	if int64(len(p)) > l.limit {
		p = p[len(p)-int(l.limit):]
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	if err != nil {
		return n, err
	}
	return original, nil
}
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
