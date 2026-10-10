//go:build windows

package install

import (
	"bytes"
	"testing"
)

func TestDPAPIBindsInstallationAndReference(t *testing.T) {
	dir := t.TempDir()
	plain := []byte("synthetic-dpapi-test-credential")
	cipher, err := protectCredential(dir, "TEST_TOKEN", plain, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipher, plain) {
		t.Fatal("credential stored in plaintext")
	}
	restored, err := protectCredential(dir, "TEST_TOKEN", cipher, true)
	if err != nil || !bytes.Equal(plain, restored) {
		t.Fatal("roundtrip failed", err)
	}
	for _, change := range []struct{ dir, name string }{{dir, "OTHER_TOKEN"}, {t.TempDir(), "TEST_TOKEN"}} {
		if _, err := protectCredential(change.dir, change.name, cipher, true); err == nil {
			t.Fatal("accepted a different credential binding")
		}
	}
	cipher[len(cipher)/2] ^= 1
	if _, err := protectCredential(dir, "TEST_TOKEN", cipher, true); err == nil {
		t.Fatal("accepted corrupted ciphertext")
	}
}
