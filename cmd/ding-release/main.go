// ding-release signs channel metadata after native package qualification. It is
// a release-engineering tool, never an end-user installation dependency.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/releasebootstrap"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/update"
)

func main() {
	var dir, out, version, channel, keyPath string
	flag.StringVar(&dir, "artifacts", "dist", "directory containing platform archives")
	flag.StringVar(&out, "out", "dist/channels", "new metadata output directory")
	flag.StringVar(&version, "version", "", "canonical release tag, including v")
	flag.StringVar(&channel, "channel", "preview", "stable or preview")
	flag.StringVar(&keyPath, "key-file", "", "private file containing a base64 Ed25519 private key")
	flag.Parse()
	if err := sign(dir, out, version, channel, keyPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sign(dir, out, version, channel, keyPath string) error {
	f, err := mcpconfig.OpenPrivate(keyPath)
	if err != nil {
		return fmt.Errorf("open protected signing key: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > 1024 {
		return fmt.Errorf("invalid signing key file")
	}
	encoded := make([]byte, info.Size())
	if _, err := f.ReadAt(encoded, 0); err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 signing key")
	}
	public := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))
	if update.PublicKey == "" || public != update.PublicKey {
		return fmt.Errorf("signing key does not match this release tool's pinned update.PublicKey")
	}
	now := time.Now().UTC().Truncate(time.Second)
	m := update.Manifest{Protocol: 1, Version: version, Channel: channel, Schema: store.SchemaVersion, PublishedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "ding_" + goos + "_" + arch + ".tar.gz"
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return fmt.Errorf("missing qualified artifact %s: %w", name, err)
			}
			hash := sha256.Sum256(data)
			m.Artifacts = append(m.Artifacts, update.Artifact{OS: goos, Arch: arch, URL: "https://github.com/ding-labs/ding/releases/download/" + version + "/" + name, SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(data))})
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n")
	if _, err := update.Verify(data, sig, public, channel, now); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	bootstrap, err := releasebootstrap.Render(m)
	if err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{channel + ".json", data}, {channel + ".sig", sig}, {channel + ".install.sh", bootstrap}} {
		f, err := os.OpenFile(filepath.Join(out, file.name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(file.data)
		closed := f.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	fmt.Printf("Signed %s %s metadata; publish channel pointers only after release qualification.\n", channel, version)
	return nil
}
