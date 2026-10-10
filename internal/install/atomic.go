package install

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// AtomicJSON replaces metadata durably. Callers must hold the installation lock.
func AtomicJSON(path string, value any) error {
	if err := mcpconfig.CheckPrivate(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	tmp := path + "." + hex.EncodeToString(id[:])
	f, err := mcpconfig.CreatePrivate(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	err = json.NewEncoder(f).Encode(value)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

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
