package install

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// Lock serializes installation mutations across processes.
func Lock(dir string) (func() error, error) {
	unlock, err := mcpconfig.Lock(filepath.Join(dir, "installation.lock"))
	if errors.Is(err, mcpconfig.ErrLocked) {
		return nil, fmt.Errorf("another installation operation is active: %w", err)
	}
	return unlock, err
}
