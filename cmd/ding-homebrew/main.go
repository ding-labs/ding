// ding-homebrew prepares a hash-pinned tap formula independently of the signed
// standalone updater. Homebrew owns distribution and updates on this route.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/releasebootstrap"
	"github.com/ding-labs/ding/internal/update"
)

func main() {
	dir := flag.String("artifacts", "dist/native", "directory containing the four native Unix archives")
	version := flag.String("version", "", "canonical release tag, including v")
	flag.Parse()
	if err := run(*dir, *version, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir, version string, out io.Writer) error {
	m := update.Manifest{Version: version, Channel: "stable"}
	for _, goos := range []string{"darwin", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "ding_" + goos + "_" + arch + ".tar.gz"
			f, err := os.Open(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			h := sha256.New()
			n, err := io.Copy(h, io.LimitReader(f, update.MaxArtifactBytes+1))
			closed := f.Close()
			if err != nil {
				return err
			}
			if closed != nil {
				return closed
			}
			if n == 0 || n > update.MaxArtifactBytes {
				return fmt.Errorf("invalid archive size: %s", name)
			}
			m.Artifacts = append(m.Artifacts, update.Artifact{OS: goos, Arch: arch,
				URL:    "https://github.com/ding-labs/ding/releases/download/" + version + "/" + name,
				SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n})
		}
	}
	_, formula, err := releasebootstrap.Homebrew(m)
	if err != nil {
		return err
	}
	_, err = out.Write(formula)
	return err
}
