package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"os"
	"path/filepath"
	"runtime"
)

func readDPAPI(dir string) (map[string][]byte, error) {
	values := map[string][]byte{}
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "dpapi.json"), &values, true)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	if len(values) > 1000 {
		return nil, fmt.Errorf("too many protected credentials")
	}
	for name, value := range values {
		if !secretName.MatchString(name) || len(name) > 256 || len(value) == 0 || len(value) > 128<<10 {
			return nil, fmt.Errorf("invalid DPAPI credential")
		}
	}
	return values, nil
}
func DPAPINames(dir string) ([]string, error) {
	values, err := readDPAPI(dir)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for name := range values {
		names = append(names, name)
	}
	return names, nil
}
func SetDPAPISecret(dir, name, value string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("DPAPI requires Windows")
	}
	if !secretName.MatchString(name) || len(name) > 256 || value == "" || len(value) > 64<<10 {
		return fmt.Errorf("invalid credential name or size")
	}
	unlock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	plain, err := ReadSecrets(dir)
	if err != nil {
		return err
	}
	native, err := KeychainNames(dir)
	if err != nil {
		return err
	}
	if plain[name] != "" || native[name] {
		return fmt.Errorf("reference already belongs to another credential backend")
	}
	values, err := readDPAPI(dir)
	if err != nil {
		return err
	}
	if len(values) >= 1000 && values[name] == nil {
		return fmt.Errorf("too many protected credentials")
	}
	cipher, err := protectCredential(dir, name, []byte(value), false)
	if err != nil {
		return err
	}
	values[name] = cipher
	encoded, err := json.Marshal(values)
	if err != nil || len(encoded) > 900<<10 {
		return fmt.Errorf("protected credential storage is full; remove unused references before adding more")
	}
	return AtomicJSON(filepath.Join(dir, "dpapi.json"), values)
}
