package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestKeychainReferenceNeverFallsBackToEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DING_TEST_KEYCHAIN_SECRET", "must-not-be-used")
	if err := AtomicJSON(filepath.Join(dir, "keychain.json"), map[string]bool{"DING_TEST_KEYCHAIN_SECRET": true}); err != nil {
		t.Fatal(err)
	}
	lookup, err := CredentialLookup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := lookup("DING_TEST_KEYCHAIN_SECRET"); ok || value != "" {
		t.Fatal("missing native reference fell back to environment")
	}
	if err := SetSecret(dir, "DING_TEST_KEYCHAIN_SECRET", "duplicate"); err == nil {
		t.Fatal("conflicting storage accepted")
	}
}

func TestNativeKeychainRoundTripAndIsolation(t *testing.T) {
	helper := os.Getenv("DING_TEST_KEYCHAIN_HELPER")
	if runtime.GOOS != "darwin" || helper == "" {
		t.Skip("opt-in native Keychain qualification")
	}
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := keychainCall(ctx, helper, dir, "set", "SYNTHETIC_TEST", "synthetic-nonproduction-value"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := keychainCall(ctx, helper, dir, "delete", "SYNTHETIC_TEST", ""); err != nil {
			t.Error("cannot remove synthetic credential", err)
		}
	}()
	value, err := keychainCall(ctx, helper, dir, "get", "SYNTHETIC_TEST", "")
	if err != nil || value != "synthetic-nonproduction-value" {
		t.Fatal("native roundtrip failed", err)
	}
	if _, err := keychainCall(ctx, helper, t.TempDir(), "get", "SYNTHETIC_TEST", ""); err == nil {
		t.Fatal("credential crossed installation identity")
	}
}
