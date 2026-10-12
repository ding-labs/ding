package cloud

import "golang.org/x/sys/windows"

func freeDiskBytes(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, available uint64
	err = windows.GetDiskFreeSpaceEx(p, &available, &total, &free)
	return available, err
}
