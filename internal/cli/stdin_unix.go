//go:build !windows

package cli

import (
	"golang.org/x/sys/unix"
	"os"
)

// Inherited stdin can be a blocking descriptor outside Go's poller. Duplicate
// it and enable nonblocking I/O before NewFile so Close can interrupt Scan.
func cancelableStdin() (*os.File, error) {
	fd, err := unix.Dup(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "ding-stdin"), nil
}
