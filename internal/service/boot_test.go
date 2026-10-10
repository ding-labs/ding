package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootTemplatesUseDedicatedIdentityAndGracefulHost(t *testing.T) {
	exe, dir := filepath.Join(os.TempDir(), "ding & binary"), filepath.Join(os.TempDir(), "ding-service-state")
	for _, platform := range []string{"darwin", "linux", "windows"} {
		d, err := BootTemplate(platform, exe, dir, "ding", "ding")
		if err != nil || d.Startup != "boot" || d.Content == "" {
			t.Fatal(platform, d, err)
		}
		switch platform {
		case "darwin":
			if !strings.Contains(d.Content, "<key>UserName</key><string>ding</string>") || !strings.Contains(d.Content, "&amp;") {
				t.Fatal(d.Content)
			}
		case "linux":
			if !strings.Contains(d.Content, "User=ding\n") || !strings.Contains(d.Content, "UMask=0077") {
				t.Fatal(d.Content)
			}
		case "windows":
			if !strings.Contains(d.Content, "service host --name") || !strings.Contains(d.Content, "Get-Credential") || !strings.Contains(d.Content, "-StartupType Automatic") {
				t.Fatal(d.Content)
			}
		}
		if _, err := BootTemplate(platform, exe, dir, "root", "root"); err == nil {
			t.Fatal("root template accepted")
		}
		if _, err := BootTemplate(platform, exe+"\nother", dir, "ding", "ding"); err == nil {
			t.Fatal("path injection accepted")
		}
	}
}
