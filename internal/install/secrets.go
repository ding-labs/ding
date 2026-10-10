package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

var secretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ReadSecrets is an explicitly local, private-file backend for headless users.
// Missing files are empty; damaged or publicly readable files fail closed.
func ReadSecrets(dir string) (map[string]string, error) {
	values := map[string]string{}
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "secrets.json"), &values, true)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot load private secrets.json: %w", err)
	}
	for name, value := range values {
		if !secretName.MatchString(name) || value == "" || len(value) > 64<<10 {
			return nil, fmt.Errorf("invalid entry in secrets.json")
		}
	}
	return values, nil
}

func SetSecret(dir, name, value string) error {
	if !secretName.MatchString(name) || value == "" || len(value) > 64<<10 {
		return fmt.Errorf("secret requires a valid environment name and 1–65536 bytes")
	}
	unlock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	native, err := KeychainNames(dir)
	if err != nil {
		return err
	}
	protected, err := readDPAPI(dir)
	if err != nil {
		return err
	}
	if protected[name] != nil {
		return fmt.Errorf("credential already uses DPAPI storage")
	}
	if native[name] {
		return fmt.Errorf("credential already uses Keychain; choose a new reference name for private-file storage")
	}
	values, err := ReadSecrets(dir)
	if err != nil {
		return err
	}
	values[name] = value
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("secret store exceeds 1 MiB")
	}
	return AtomicJSON(filepath.Join(dir, "secrets.json"), values)
}
