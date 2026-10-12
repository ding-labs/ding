package mcpconfig

import (
	"errors"
	"fmt"
	"os"
)

// Lock serializes private file mutations. OS locks release on process death;
// the persistent lock file must never be removed while another process uses it.
func Lock(path string) (func() error, error) {
	f, err := CreatePrivate(path)
	if err == nil {
		err = f.Close()
	}
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	f, err = OpenPrivate(path)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another operation on this private file is active: %w", err)
	}
	return f.Close, nil
}
