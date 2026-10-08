package cli

import (
	"golang.org/x/sys/windows"
	"os"
)

func cancelableStdin() (*os.File, error) {
	var handle windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(os.Stdin.Fd()), process, &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), "ding-stdin"), nil
}
