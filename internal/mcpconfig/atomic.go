package mcpconfig

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// AtomicJSON replaces metadata durably. Callers must hold the private file lock.
func AtomicJSON(path string, value any) error {
	if err := CheckPrivate(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	tmp := path + "." + hex.EncodeToString(id[:])
	f, err := CreatePrivate(tmp)
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
