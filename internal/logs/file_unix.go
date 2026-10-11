//go:build !windows

package logs

import (
	"fmt"
	"os"
	"syscall"
)

func openLogFile(path string) (*os.File, error) {
	// Nonblocking open also prevents a racing replacement with a FIFO from
	// hanging the daemon before the regular-file check.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func fileIdentity(info os.FileInfo) string {
	s := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d", s.Dev, s.Ino)
}
