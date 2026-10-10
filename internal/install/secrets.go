package install

import (
	"crypto/rand"
	"encoding/hex"
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
	// Reserve a short-lived writer lock rather than lose concurrent edits.
	lock, err := mcpconfig.CreatePrivate(filepath.Join(dir, "secrets.edit.lock"))
	if err != nil {
		return fmt.Errorf("secret update unavailable; another writer or an interrupted edit holds secrets.edit.lock: %w", err)
	}
	defer os.Remove(lock.Name())
	defer lock.Close()
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
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	f, err := mcpconfig.CreatePrivate(filepath.Join(dir, ".secrets-"+hex.EncodeToString(nonce[:])+".json"))
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "secrets.json"))
}
