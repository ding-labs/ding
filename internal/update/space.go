package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CheckSpace reserves a conservative expansion bound plus database/WAL backup
// space. The filesystem can still fill afterward; every write must also fail
// safely. No network or daemon mutation occurs during this preflight.
func CheckSpace(binaryDir, stateDir string, compressedBytes int64) error {
	if compressedBytes <= 0 || compressedBytes > MaxArtifactBytes {
		return fmt.Errorf("invalid update artifact size")
	}
	var databaseBytes uint64
	for _, name := range []string{"ding.db", "ding.db-wal"} {
		info, err := os.Lstat(filepath.Join(stateDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return fmt.Errorf("invalid database backup source")
		}
		databaseBytes += uint64(info.Size())
	}
	// Check the combined amount at both locations: conservative if they are
	// different filesystems, necessary if they share the same filesystem.
	required := uint64(compressedBytes+MaxArtifactBytes+64<<20) + databaseBytes
	for _, dir := range []string{binaryDir, stateDir} {
		available, err := freeBytes(dir)
		if err != nil {
			return fmt.Errorf("cannot check update disk space: %w", err)
		}
		if available < required {
			return fmt.Errorf("insufficient disk space at %s: need %d bytes for staging and backup, available %d", dir, required, available)
		}
	}
	return nil
}
