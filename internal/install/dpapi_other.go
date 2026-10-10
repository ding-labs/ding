//go:build !windows

package install

import "fmt"

func protectCredential(string, string, []byte, bool) ([]byte, error) {
	return nil, fmt.Errorf("DPAPI credentials require the owning Windows profile; rebind them on this machine")
}
