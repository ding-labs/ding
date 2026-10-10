package releasebootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/update"
)

func TestHomebrewFormulaRequiresEveryQualifiedNativePlatform(t *testing.T) {
	m := update.Manifest{Version: "v1.0.0-preview.1", Channel: "preview"}
	for _, goos := range []string{"darwin", "linux"} {
		for _, arch := range []string{"arm64", "amd64"} {
			m.Artifacts = append(m.Artifacts, update.Artifact{OS: goos, Arch: arch, URL: "https://github.com/ding-labs/ding/releases/download/" + m.Version + "/ding_" + goos + "_" + arch + ".tar.gz", SHA256: strings.Repeat("a", 64)})
		}
	}
	name, formula, err := Homebrew(m)
	if err != nil || name != "ding-preview.rb" {
		t.Fatal(name, err)
	}
	if path, err := exec.LookPath("ruby"); err == nil {
		file := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(file, formula, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(path, "-c", file).CombinedOutput(); err != nil {
			t.Fatal("invalid formula syntax", string(out), err)
		}
	}
	m.Artifacts[0].URL += `"; system("unexpected")`
	if _, _, err := Homebrew(m); err == nil {
		t.Fatal("accepted substituted artifact")
	}
	m.Artifacts = m.Artifacts[1:]
	if _, _, err := Homebrew(m); err == nil {
		t.Fatal("accepted incomplete platform matrix")
	}
}
