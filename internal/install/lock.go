package install

import (
	"github.com/ding-labs/ding/internal/mcpconfig"
	"path/filepath"
)

// Lock serializes installation mutations across processes.
func Lock(dir string) (func() error, error) {
	return mcpconfig.Lock(filepath.Join(dir, "installation.lock"))
}
