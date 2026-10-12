//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ding-labs/ding/internal/railway"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "run" {
		if os.Geteuid() != railway.UID || os.Getegid() != railway.UID {
			return fmt.Errorf("server must run as UID/GID 10001")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return railway.Run(ctx, os.Stderr)
	}
	if len(os.Args) != 1 || os.Geteuid() != 0 {
		return fmt.Errorf("Railway entrypoint requires root for volume setup only")
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil || !railway.Mounted(string(mounts)) || os.Getenv("RAILWAY_VOLUME_MOUNT_PATH") != railway.VolumeDir {
		return fmt.Errorf("Railway persistent volume must be mounted at /data; refusing ephemeral startup")
	}
	s, key, password, err := railway.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	if err := railway.Prepare(s, key, password); err != nil {
		return err
	}
	// exec removes secret input from the serving process environment, including
	// its initial /proc environment. Secrets remain in owner-only runtime files.
	env := []string{}
	for _, pair := range os.Environ() {
		name, _, _ := strings.Cut(pair, "=")
		if railway.SecretEnvironment(name) {
			continue
		}
		env = append(env, pair)
	}
	return syscall.Exec("/sbin/su-exec", []string{"su-exec", "10001:10001", "/ding-cloud-railway", "run"}, env)
}
