package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Client struct{ HTTP *http.Client }

func (c Client) get(ctx context.Context, address string) (*http.Response, error) {
	h := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		h = *c.HTTP
	}
	h.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Scheme != "https" {
			return fmt.Errorf("unsafe release redirect")
		}
		switch r.URL.Hostname() {
		case "github.com", "raw.githubusercontent.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			return nil
		}
		return fmt.Errorf("unexpected release download host")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("updates require HTTPS")
	}
	response, err := h.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, fmt.Errorf("release download unavailable (HTTP %d)", response.StatusCode)
	}
	return response, nil
}

func (c Client) Check(ctx context.Context, key, channel string, now time.Time) (Manifest, error) {
	if channel != "stable" && channel != "preview" {
		return Manifest{}, fmt.Errorf("invalid update channel")
	}
	if key == "" {
		return Manifest{}, fmt.Errorf("updates are not configured in this development build")
	}
	base := "https://raw.githubusercontent.com/ding-labs/ding/main/releases/channels/" + channel
	read := func(address string, max int64) ([]byte, error) {
		r, err := c.get(ctx, address)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		b, err := io.ReadAll(io.LimitReader(r.Body, max+1))
		if err == nil && int64(len(b)) > max {
			err = fmt.Errorf("release metadata too large")
		}
		return b, err
	}
	data, err := read(base+".json", 1<<20)
	if err != nil {
		return Manifest{}, err
	}
	sig, err := read(base+".sig", 1024)
	if err != nil {
		return Manifest{}, err
	}
	return Verify(data, sig, key, channel, now)
}

// Stage verifies the complete archive digest before extracting any executable.
// The caller owns the private, unique staging directory and its cleanup.
func (c Client) Stage(ctx context.Context, a Artifact, dir string) (string, error) {
	r, err := c.get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	archive, err := os.OpenFile(filepath.Join(dir, "artifact.tar.gz"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(r.Body, a.Bytes+1))
	closeErr := archive.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n != a.Bytes || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return "", fmt.Errorf("release archive size or digest mismatch")
	}
	return extract(filepath.Join(dir, "artifact.tar.gz"), dir, a.OS)
}

func extract(archive, dir, goos string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	t := tar.NewReader(gz)
	var size int64
	binary := "ding"
	if goos == "windows" {
		binary += ".exe"
	}
	seen := map[string]bool{}
	for {
		header, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") || name == ".." || strings.HasPrefix(name, "../") {
			return "", fmt.Errorf("unsafe release archive path")
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
			return "", fmt.Errorf("release archive contains a non-regular entry")
		}
		if header.Size < 0 || header.Size > MaxArtifactBytes-size {
			return "", fmt.Errorf("release archive expands beyond its limit")
		}
		size += header.Size
		wanted := name == binary || (goos == "darwin" && (name == "DingNotifications.app" || strings.HasPrefix(name, "DingNotifications.app/")))
		if !wanted {
			continue
		}
		if seen[name] {
			return "", fmt.Errorf("duplicate release archive entry")
		}
		seen[name] = true
		target := filepath.Join(dir, filepath.FromSlash(name))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		mode := os.FileMode(0600)
		if name == binary || strings.Contains(name, "/Contents/MacOS/") {
			mode = 0700
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(out, t)
		if err == nil {
			err = out.Sync()
		}
		closed := out.Close()
		if err != nil {
			return "", err
		}
		if closed != nil {
			return "", closed
		}
	}
	if !seen[binary] {
		return "", fmt.Errorf("release archive has no Ding executable")
	}
	if goos == "darwin" && (!seen["DingNotifications.app/Contents/MacOS/DingNotifications"] || !seen["DingNotifications.app/Contents/Info.plist"]) {
		return "", fmt.Errorf("macOS release is missing its native notification helper")
	}
	return filepath.Join(dir, binary), nil
}
