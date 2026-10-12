package install

import (
	"errors"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"path/filepath"
)

// AtomicJSON replaces private metadata. Callers must hold the installation lock.
func AtomicJSON(path string, value any) error { return mcpconfig.AtomicJSON(path, value) }

// Replace refreshes an owned installation after its executable changes.
func Replace(r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	old, err := Load(r.StateDir)
	if err != nil {
		return err
	}
	if old.Executable != r.Executable || old.Owner != r.Owner {
		return errors.New("installation ownership changed")
	}
	return AtomicJSON(filepath.Join(r.StateDir, RecordName), r)
}
