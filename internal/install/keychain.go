package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/nativehelper"
	"github.com/ding-labs/ding/internal/source"
)

func KeychainNames(dir string) (map[string]bool, error) {
	names := map[string]bool{}
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "keychain.json"), &names, true)
	if errors.Is(err, os.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	if len(names) > 1000 {
		return nil, fmt.Errorf("too many credential references")
	}
	for name, enabled := range names {
		if !enabled || !secretName.MatchString(name) || len(name) > 256 {
			return nil, fmt.Errorf("invalid Keychain reference")
		}
	}
	return names, nil
}

func keychainCall(ctx context.Context, path, dir, operation, name, value string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	id := sha256.Sum256([]byte(dir))
	data, err := json.Marshal(map[string]string{"operation": operation, "service": "ing.ding.credentials." + hex.EncodeToString(id[:]), "name": name, "value": value})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--credentials")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil || len(out) > 64<<10 {
		return "", fmt.Errorf("Keychain unavailable, locked, or credential missing; unlock the owning user session")
	}
	return string(out), nil
}

func SetKeychainSecret(ctx context.Context, dir, name, value string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("Keychain storage requires macOS; explicitly choose private-file for headless operation")
	}
	if !secretName.MatchString(name) || len(name) > 256 || value == "" || len(value) > 64<<10 {
		return fmt.Errorf("invalid secret name or size")
	}
	unlock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	files, err := ReadSecrets(dir)
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
	if _, ok := files[name]; ok {
		return fmt.Errorf("this credential already uses private-file storage; choose a new reference name to migrate explicitly")
	}
	names, err := KeychainNames(dir)
	if err != nil {
		return err
	}
	if !names[name] && len(names) >= 1000 {
		return fmt.Errorf("too many credential references")
	}
	path, err := nativehelper.Path()
	if err != nil {
		return err
	}
	if _, err := keychainCall(ctx, path, dir, "set", name, value); err != nil {
		return err
	}
	names[name] = true
	return AtomicJSON(filepath.Join(dir, "keychain.json"), names)
}

// CredentialLookup snapshots explicit reference names, never falls back to an
// environment variable for a missing Keychain reference, and retries locked
// Keychains after a short cache interval without prompting from a background job.
func CredentialLookup(dir string) (source.Lookup, error) {
	files, err := ReadSecrets(dir)
	if err != nil {
		return nil, err
	}
	names, err := KeychainNames(dir)
	if err != nil {
		return nil, err
	}
	protected, err := readDPAPI(dir)
	if err != nil {
		return nil, err
	}
	for name := range protected {
		if _, ok := files[name]; ok || names[name] {
			return nil, fmt.Errorf("credential %s has conflicting storage backends", name)
		}
	}
	for name := range names {
		if _, ok := files[name]; ok {
			return nil, fmt.Errorf("credential %s has conflicting storage backends", name)
		}
	}
	type cached struct {
		value string
		until time.Time
	}
	var mu sync.Mutex
	cache := map[string]cached{}
	return func(name string) (string, bool) {
		if value, ok := files[name]; ok {
			return value, true
		}
		if cipher, ok := protected[name]; ok {
			value, err := protectCredential(dir, name, cipher, true)
			return string(value), err == nil && len(value) > 0
		}
		if !names[name] {
			return os.LookupEnv(name)
		}
		mu.Lock()
		defer mu.Unlock()
		if entry, ok := cache[name]; ok && time.Now().Before(entry.until) {
			return entry.value, entry.value != ""
		}
		value := ""
		if runtime.GOOS == "darwin" {
			if path, err := nativehelper.Path(); err == nil {
				value, _ = keychainCall(context.Background(), path, dir, "get", name, "")
			}
		}
		cache[name] = cached{value, time.Now().Add(30 * time.Second)}
		return value, value != ""
	}, nil
}
