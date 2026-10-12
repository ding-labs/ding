//go:build !darwin && !linux && !windows

package cloud

import "fmt"

func freeDiskBytes(string) (uint64, error) {
	return 0, fmt.Errorf("cloud push disk reserve check is unsupported on this OS")
}
