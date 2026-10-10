package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func tarball(t *testing.T, name string, kind byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: name, Mode: 0700, Size: 4, Typeflag: kind}
	if kind != tar.TypeReg {
		h.Size = 0
		h.Linkname = "/tmp/evil"
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		_, _ = tw.Write([]byte("ding"))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestStageRejectsTamperingAndArchiveEscapes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   byte
		tamper bool
		ok     bool
	}{{"ding", tar.TypeReg, false, true}, {"../escape", tar.TypeReg, false, false}, {"/tmp/escape", tar.TypeReg, false, false}, {"ding", tar.TypeSymlink, false, false}, {"ding", tar.TypeReg, true, false}} {
		b := tarball(t, tc.name, tc.kind)
		h := sha256.Sum256(b)
		a := Artifact{OS: "linux", Arch: "amd64", URL: "https://github.com/ding-labs/ding/releases/download/v1.0.0/ding.tar.gz", Bytes: int64(len(b)), SHA256: hex.EncodeToString(h[:])}
		if tc.tamper {
			b[0] ^= 1
		}
		c := Client{HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
		})}}
		dir := t.TempDir()
		exe, err := c.Stage(context.Background(), a, dir)
		if tc.ok {
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(exe); err != nil || string(data) != "ding" {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("accepted unsafe artifact", tc)
		}
		if _, err := os.Stat(filepath.Join(dir, "escape")); err == nil {
			t.Fatal("archive escaped filter")
		}
	}
}
