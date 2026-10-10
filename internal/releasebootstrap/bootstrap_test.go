//go:build !windows

package releasebootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/update"
)

func TestBootstrapInstallsVerifiedArchiveAndRefusesOverwrite(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	binary := []byte("#!/bin/sh\necho ding version 1.0.0\n")
	if err := tw.WriteHeader(&tar.Header{Name: "ding", Mode: 0755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	hash := sha256.Sum256(archive.Bytes())
	m := update.Manifest{Version: "v1.0.0", Channel: "preview", Artifacts: []update.Artifact{{OS: "linux", Arch: "amd64", URL: "https://github.com/ding-labs/ding/releases/download/v1.0.0/ding_linux_amd64.tar.gz", SHA256: hex.EncodeToString(hash[:])}}}
	script, err := Render(m)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mocks := filepath.Join(root, "mocks")
	dest := filepath.Join(root, "bin")
	if err := os.Mkdir(mocks, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path string, data []byte, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "artifact"), archive.Bytes(), 0600)
	write(filepath.Join(root, "install.sh"), script, 0700)
	write(filepath.Join(mocks, "uname"), []byte("#!/bin/sh\ncase $1 in -s) echo Linux;; -m) echo x86_64;; esac\n"), 0700)
	write(filepath.Join(mocks, "curl"), []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then cp \"$DING_TEST_ARCHIVE\" \"$2\"; exit; fi; shift; done\nexit 1\n"), 0700)
	run := func() error {
		c := exec.Command("sh", filepath.Join(root, "install.sh"))
		c.Env = append(os.Environ(), "PATH="+mocks+":"+os.Getenv("PATH"), "INSTALL_DIR="+dest, "DING_TEST_ARCHIVE="+filepath.Join(root, "artifact"))
		b, err := c.CombinedOutput()
		t.Log(string(b))
		return err
	}
	if out, err := exec.Command("sh", "-n", filepath.Join(root, "install.sh")).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "ding"))
	if err != nil || !bytes.Equal(got, binary) {
		t.Fatal("wrong executable", err)
	}
	if err := run(); err == nil {
		t.Fatal("overwrote an existing installation")
	}
	if err := os.Remove(filepath.Join(dest, "ding")); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(root, "artifact"), []byte("tampered"), 0600)
	if err := run(); err == nil {
		t.Fatal("accepted a tampered artifact")
	}
	if _, err := os.Stat(filepath.Join(dest, "ding")); !os.IsNotExist(err) {
		t.Fatal("installed before verifying")
	}
}
