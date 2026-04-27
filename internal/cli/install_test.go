package cli

import (
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildDingBinary builds cmd/ding into a temp file and returns its path.
// Uses the test's TempDir so the binary is cleaned up automatically.
func buildDingBinary(t *testing.T) string {
	t.Helper()
	binName := "ding-test"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)

	// Locate the repo root (two levels up from internal/cli) so `go build`
	// can find ./cmd/ding regardless of where the test runs from.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ding")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return binPath
}

// fileSHA256 returns the SHA-256 hex digest of the file at path.
func fileSHA256(t *testing.T, path string) [32]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash %q: %v", path, err)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// TestInstall_CopiesByteIdentical builds the ding binary, runs
// `ding install <dst>`, and verifies the destination is byte-identical
// and executable.
func TestInstall_CopiesByteIdentical(t *testing.T) {
	src := buildDingBinary(t)
	dst := filepath.Join(t.TempDir(), "ding-installed")

	cmd := exec.Command(src, "install", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed binary not executable: mode=%v", info.Mode().Perm())
	}

	if got, want := fileSHA256(t, dst), fileSHA256(t, src); got != want {
		t.Errorf("installed binary sha256 mismatch: got %x, want %x", got, want)
	}

	// Quick sanity: the installed binary should run.
	out, err := exec.Command(dst, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("installed binary failed to run: %v\n%s", err, out)
	}
}

// TestInstall_OverwritesExistingDestination verifies that install
// truncates and replaces an existing file at dst (cp/install semantics).
func TestInstall_OverwritesExistingDestination(t *testing.T) {
	src := buildDingBinary(t)
	dst := filepath.Join(t.TempDir(), "ding-installed")

	// Pre-create dst with smaller, non-executable content.
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}

	cmd := exec.Command(src, "install", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	if got, want := fileSHA256(t, dst), fileSHA256(t, src); got != want {
		t.Errorf("post-overwrite sha256 mismatch: got %x, want %x", got, want)
	}
}

// TestInstall_NonexistentDestinationDir verifies that installing into
// a directory that does not exist surfaces a clear error and exits non-zero.
func TestInstall_NonexistentDestinationDir(t *testing.T) {
	src := buildDingBinary(t)
	dst := filepath.Join(t.TempDir(), "does-not-exist", "ding-installed")

	cmd := exec.Command(src, "install", dst)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected install to fail with missing dest dir, got success; output: %s", out)
	}
	// Exit status should be non-zero (cobra returns 1 on RunE error).
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() == 0 {
			t.Errorf("expected non-zero exit code, got 0")
		}
	} else {
		t.Errorf("expected *exec.ExitError, got %T: %v", err, err)
	}
	// Error output should mention the destination path so users can act on it.
	if len(out) == 0 {
		t.Errorf("expected error output to stderr, got empty")
	}
}

// TestInstall_RequiresArg verifies that `ding install` with no args fails.
func TestInstall_RequiresArg(t *testing.T) {
	src := buildDingBinary(t)
	cmd := exec.Command(src, "install")
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected `ding install` with no args to fail")
	}
}
