// Package pluginpackage assembles qualification artifacts for official host
// review. Assembly never publishes, signs, or installs a personal marketplace.
package pluginpackage

import (
	"archive/zip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Options struct{ Root, Mode, Runtime, Target, Endpoint, Output, Version string }

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$`)

func Build(o Options) (string, error) {
	if o.Root == "" {
		o.Root = "."
	}
	if o.Output == "" || !versionPattern.MatchString(o.Version) {
		return "", errors.New("output and a semantic version are required")
	}
	platform := "claude"
	native := o.Mode == "native-claude"
	executable := "ding-mcp"
	if o.Mode == "remote-chatgpt" {
		platform = "chatgpt"
	} else if o.Mode != "remote-claude" && !native {
		return "", errors.New("unsupported package mode")
	}
	if native {
		switch o.Target {
		case "darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64", "windows-amd64":
		default:
			return "", errors.New("unsupported native target")
		}
		if strings.HasPrefix(o.Target, "windows-") {
			executable += ".exe"
		}
		if err := validateRuntime(filepath.Join(o.Runtime, executable), o.Target); err != nil {
			return "", err
		}
	} else {
		u, err := url.Parse(o.Endpoint)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "/mcp" {
			return "", errors.New("supply a credential-free HTTPS endpoint ending in /mcp")
		}
	}
	// Refuse replacement of a previous artifact or an interrupted build.
	if err := os.MkdirAll(filepath.Dir(o.Output), 0755); err != nil {
		return "", err
	}
	if err := os.Mkdir(o.Output, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(o.Output, "ding")
	for from, to := range map[string]string{filepath.Join("plugins", platform, "ding"): "", "plugins/shared/skills": "skills", "plugins/shared/assets": "assets"} {
		if err := copyTree(filepath.Join(o.Root, from), filepath.Join(dest, to)); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"privacy.md", "release.md", "verification.md"} {
		if err := copyFile(filepath.Join(o.Root, "docs/integrations", name), filepath.Join(dest, name), 0644); err != nil {
			return "", err
		}
	}
	if err := copyFile(filepath.Join(o.Root, "LICENSE"), filepath.Join(dest, "LICENSE"), 0644); err != nil {
		return "", err
	}
	manifestPath := filepath.Join(dest, ".claude-plugin/plugin.json")
	if platform == "chatgpt" {
		manifestPath = filepath.Join(dest, "plugin.json")
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return "", err
	}
	manifest["version"] = o.Version
	if err := writeJSON(manifestPath, manifest); err != nil {
		return "", err
	}
	config := map[string]any{}
	var server map[string]any
	var readme string
	if native {
		if err := copyFile(filepath.Join(o.Runtime, executable), filepath.Join(dest, "runtime/ding-mcp", executable), 0755); err != nil {
			return "", err
		}
		server = map[string]any{"command": "${CLAUDE_PLUGIN_ROOT}/runtime/ding-mcp/" + executable, "args": []string{"serve"}}
		if err := launcher(dest, o.Target, o.Version); err != nil {
			return "", err
		}
		readme = "Open Ding Setup to pair with an existing running Ding daemon, then enable/restart this plugin in local Claude Cowork or Claude Code. This native package does not run inside ordinary Claude chat. No Python or Node installation is needed."
	} else {
		typeName := "http"
		if platform == "chatgpt" {
			typeName = "streamable-http"
		}
		server = map[string]any{"type": typeName, "url": o.Endpoint}
		if platform == "chatgpt" {
			server["extensions"] = map[string]any{"com.openai": map[string]any{"auth": map[string]any{"type": "oauth", "baseScopes": []string{"ding:inspect", "ding:preview", "ding:manage", "ding:retry"}}}}
			config["$schema"] = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
		}
		readme = "Connect through your host's OAuth flow. This package targets one operator-supplied, self-hosted endpoint. An operator must pair your identity to a Ding grant. It does not route arbitrary customer endpoints or expose localhost."
	}
	config["mcpServers"] = map[string]any{"ding": server}
	configName := ".mcp.json"
	if platform == "chatgpt" {
		configName = "mcp.json"
	}
	if err := writeJSON(filepath.Join(dest, configName), config); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("# Ding\n\n"+readme+"\n\nThis is a qualification build, not evidence of official marketplace approval.\n\nAuthoritative data remains on your Ding instance; data you request is sent to your selected LLM provider. See the included privacy and review documents.\n"), 0644); err != nil {
		return "", err
	}
	archive := filepath.Join(o.Output, "ding.zip")
	if err := archiveTree(dest, archive); err != nil {
		return "", err
	}
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	f.Close()
	if err != nil {
		return "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if err := os.WriteFile(filepath.Join(o.Output, "SHA256SUMS"), []byte(digest+"  ding.zip\n"), 0644); err != nil {
		return "", err
	}
	err = writeJSON(filepath.Join(o.Output, "qualification.json"), map[string]any{"mode": o.Mode, "target": o.Target, "version": o.Version, "sha256": digest, "signed": false, "marketplaceApproved": false, "containsRuntime": native, "runtime": "go"})
	return archive, err
}

func validateRuntime(path, target string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return errors.New("runtime must contain the native Go ding-mcp executable")
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	parts := strings.SplitN(target, "-", 2)
	// Trimpath builds omit linker flags from build info. CI checks --version by
	// running the extracted artifact on each target instead of inferring it here.
	if info.Path != "github.com/ding-labs/ding/cmd/ding-mcp" || settings["GOOS"] != parts[0] || settings["GOARCH"] != parts[1] || !slices.Contains(strings.Split(settings["-tags"], ","), "mcpui") {
		return errors.New("native runtime target or UI build tag does not match package")
	}
	return nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func copyFile(from, to string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return err
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("package input must be a regular file")
	}
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(target, source)
	closeErr := target.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlinks are not accepted in plugin templates")
		}
		return copyFile(path, dest, 0644)
	})
}
func launcher(dest, target, version string) error {
	write := func(path, body string, mode fs.FileMode) error {
		path = filepath.Join(dest, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(body), mode)
	}
	if strings.HasPrefix(target, "darwin-") {
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleName</key><string>Ding Setup</string><key>CFBundleIdentifier</key><string>ing.ding.mcp.setup</string><key>CFBundleVersion</key><string>%s</string><key>CFBundleExecutable</key><string>setup</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`, version)
		if err := write("Ding Setup.app/Contents/Info.plist", plist, 0644); err != nil {
			return err
		}
		return write("Ding Setup.app/Contents/MacOS/setup", "#!/bin/sh\nHERE=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\"\nexec \"$HERE/../../../runtime/ding-mcp/ding-mcp\" setup\n", 0755)
	}
	if strings.HasPrefix(target, "windows-") {
		return write("Ding Setup.cmd", "@echo off\r\n\"%~dp0runtime\\ding-mcp\\ding-mcp.exe\" setup\r\n", 0644)
	}
	return write("Ding Setup.sh", "#!/bin/sh\nHERE=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\"\nexec \"$HERE/runtime/ding-mcp/ding-mcp\" setup\n", 0755)
}
func archiveTree(source, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Method = zip.Deflate
		h.Modified = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		_, err = io.Copy(w, input)
		return err
	})
	zErr := z.Close()
	fErr := f.Close()
	if err != nil {
		return err
	}
	if zErr != nil {
		return zErr
	}
	return fErr
}
