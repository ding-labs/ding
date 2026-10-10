// Package install records local installation ownership independently of a daemon.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

const RecordName = "installation.json"

type Record struct {
	Schema     int       `json:"schema"`
	Executable string    `json:"executable"`
	StateDir   string    `json:"stateDir"`
	Owner      string    `json:"owner"`
	Version    string    `json:"version"`
	Channel    string    `json:"channel"`
	Digest     string    `json:"sha256"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (r Record) Validate() error {
	if r.Schema != 1 || !filepath.IsAbs(r.Executable) || !filepath.IsAbs(r.StateDir) ||
		strings.ContainsAny(r.Executable+r.StateDir, "\x00\r\n") || r.Version == "" {
		return fmt.Errorf("invalid installation record; preserve it and repair explicitly")
	}
	if r.Owner != "standalone" && r.Owner != "homebrew" && r.Owner != "external" {
		return fmt.Errorf("unknown installation owner")
	}
	if r.Channel != "stable" && r.Channel != "preview" && r.Channel != "development" {
		return fmt.Errorf("unknown release channel")
	}
	digest, err := hex.DecodeString(r.Digest)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid installation digest")
	}
	return nil
}

func Load(dir string) (Record, error) {
	var r Record
	if err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, RecordName), &r, true); err != nil {
		return r, err
	}
	if err := r.Validate(); err != nil {
		return r, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil || filepath.Clean(r.StateDir) != abs {
		return r, fmt.Errorf("installation belongs to a different state directory")
	}
	return r, nil
}

// Create never overwrites an existing installation, including a damaged record.
func Create(r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	old, err := Load(r.StateDir)
	if err == nil {
		if old.Executable != r.Executable || old.Owner != r.Owner {
			return fmt.Errorf("state already belongs to another installation (%s); use its executable", old.Owner)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := mcpconfig.CreatePrivate(filepath.Join(r.StateDir, RecordName))
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(r)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func Inspect(executable, dir, version, owner string) (Record, error) {
	var r Record
	path, err := filepath.Abs(executable)
	if err != nil {
		return r, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return r, err
	}
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return r, fmt.Errorf("executable must be a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return r, err
	}
	channel := "stable"
	if version == "dev" || version == "reference" {
		channel = "development"
	} else if strings.Contains(version, "-") {
		channel = "preview"
	}
	r = Record{Schema: 1, Executable: path, StateDir: dir, Owner: owner, Version: version, Channel: channel, Digest: hex.EncodeToString(h.Sum(nil)), CreatedAt: time.Now().UTC()}
	return r, r.Validate()
}
