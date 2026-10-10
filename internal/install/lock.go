package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// Lock serializes installation mutations. OS locks release on process death;
// the persistent lock file must never be removed while another process uses it.
func Lock(dir string) (func() error, error) {
	path := filepath.Join(dir, "installation.lock")
	f, err := mcpconfig.CreatePrivate(path)
	if err == nil {
		err = f.Close()
	}
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	f, err = mcpconfig.OpenPrivate(path)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another installation operation is active: %w", err)
	}
	return f.Close, nil
}
