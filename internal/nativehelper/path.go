// Package nativehelper locates the signed helper shipped beside the Go binary.
package nativehelper

import (
	"fmt"
	"os"
	"path/filepath"
)

func Path() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	base := filepath.Dir(exe)
	for _, path := range []string{filepath.Join(base, "DingNotifications.app", "Contents", "MacOS", "DingNotifications"), filepath.Join(base, "..", "libexec", "ding", "DingNotifications.app", "Contents", "MacOS", "DingNotifications")} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", fmt.Errorf("Ding native helper is missing; install the complete macOS package")
}
