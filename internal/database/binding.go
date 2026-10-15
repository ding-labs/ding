package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

const BindingFile = "database-backend.json"

type Binding struct {
	Version  int    `json:"version"`
	Backend  string `json:"backend"`
	Instance string `json:"instance"`
}

// CheckDirectory prevents an existing installation from silently opening an
// empty replacement backend. Backend selection never transfers existing data.
func CheckDirectory(dir, backend string) error {
	var b Binding
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, BindingFile), &b, true)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if b.Version != 1 || b.Backend != backend {
		return fmt.Errorf("data directory belongs to a different database backend; use the explicit migration workflow")
	}
	return nil
}

func BindDirectory(dir, backend, instance string) error {
	if err := CheckDirectory(dir, backend); err != nil {
		return err
	}
	path := filepath.Join(dir, BindingFile)
	var old Binding
	if err := mcpconfig.ReadPrivateJSON(path, &old, true); err == nil {
		if old.Instance != instance {
			return fmt.Errorf("database instance differs from the data directory binding")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := mcpconfig.CreatePrivate(path)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if err = json.NewEncoder(f).Encode(Binding{Version: 1, Backend: backend, Instance: instance}); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
