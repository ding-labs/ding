// Package railway provides the deliberately narrow container startup profile.
package railway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/serverops"
)

const RuntimeDir = "/run/ding"
const VolumeDir = "/data"
const UID = 10001

type Settings struct {
	Config    cloud.Config `json:"config"`
	Mode      string       `json:"mode"`
	Username  string       `json:"username,omitempty"`
	OwnerName string       `json:"ownerName,omitempty"`
}

// FromEnv fixes hosting and admission policy; environment variables cannot turn
// this image into a self-hosted runner or enable commands. AuthKit enrollment
// requires its explicit, bounded signup limit.
func FromEnv(get func(string) string) (Settings, []byte, string, error) {
	s := Settings{Mode: get("DING_MODE"), Username: get("DING_BOOTSTRAP_USERNAME"), OwnerName: get("DING_BOOTSTRAP_NAME")}
	if s.Mode == "" {
		s.Mode = "serve"
	}
	if s.Mode != "serve" && s.Mode != "init" && s.Mode != "maintenance" {
		return s, nil, "", fmt.Errorf("DING_MODE must be serve, init or maintenance")
	}
	password := get("DING_BOOTSTRAP_PASSWORD")
	if s.Mode == "init" {
		if s.Username == "" || len(password) < 12 || len(password) > 1024 {
			return s, nil, "", fmt.Errorf("init requires DING_BOOTSTRAP_USERNAME and a 12–1024 byte DING_BOOTSTRAP_PASSWORD")
		}
		if s.OwnerName == "" {
			s.OwnerName = "Owner"
		}
	} else if password != "" || s.Username != "" || s.OwnerName != "" {
		return s, nil, "", fmt.Errorf("remove bootstrap variables before ordinary startup")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(get("DING_DATA_KEY_BASE64"))
	if err != nil || len(key) != 32 {
		return s, nil, "", fmt.Errorf("DING_DATA_KEY_BASE64 must encode exactly 32 bytes; never replace a lost key")
	}
	port, err := strconv.Atoi(get("PORT"))
	if err != nil || port < 1024 || port > 65535 {
		return s, nil, "", fmt.Errorf("PORT must be 1024–65535")
	}
	stateName := get("DING_STATE_NAME")
	if stateName == "" {
		stateName = "ding"
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`).MatchString(stateName) || stateName == "backups" {
		return s, nil, "", fmt.Errorf("DING_STATE_NAME must be a simple directory name, excluding backups")
	}
	closed := 0
	s.Config = cloud.Config{Hosting: "cloud", Deployment: "railway", LoginMethods: []string{"password"}, Name: get("DING_NAME"), PublicURL: get("DING_PUBLIC_URL"), Listen: fmt.Sprintf("0.0.0.0:%d", port), OperatorListen: "127.0.0.1:8788", DataDir: filepath.Join(VolumeDir, stateName), KeyFile: RuntimeDir + "/data.key", EnrollmentLimit: &closed}
	if port == 8788 {
		return s, nil, "", fmt.Errorf("PORT 8788 is reserved for private operator metrics")
	}
	if err := configureMCP(&s.Config, get); err != nil {
		return s, nil, "", err
	}
	return s, key, password, s.Config.Validate()
}

// Mounted rejects an ephemeral /data directory. Railway volumes mount only at
// runtime, so this check intentionally cannot be used in a pre-deploy command.
func Mounted(mountinfo string) bool {
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 6 && fields[4] == VolumeDir {
			return true
		}
	}
	return false
}

// Prepare runs as root for initial Railway volume ownership only. It never walks
// the database tree or changes existing private files' ownership/permissions.
func Prepare(s Settings, key []byte, password string) error {
	info, err := os.Lstat(VolumeDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("/data must be a mounted directory")
	}
	for _, dir := range []string{s.Config.DataDir, VolumeDir + "/backups", RuntimeDir} {
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("%s must be a private real directory", dir)
		}
		if err := os.Chown(dir, UID, UID); err != nil {
			return err
		}
	}
	// Permit the unprivileged operator to create fresh restore directories on
	// this volume without granting any access to other container users.
	if err := os.Chown(VolumeDir, UID, UID); err != nil {
		return err
	}
	if err := os.Chmod(VolumeDir, 0700); err != nil {
		return err
	}
	config, err := json.Marshal(s.Config)
	if err != nil {
		return err
	}
	settings, err := json.Marshal(s)
	if err != nil {
		return err
	}
	files := map[string][]byte{"data.key": key, "config.json": config, "startup.json": settings}
	if s.Mode == "init" {
		files["bootstrap.password"] = []byte(password)
	}
	for name, data := range files {
		f, err := os.CreateTemp(RuntimeDir, ".startup-")
		if err != nil {
			return err
		}
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		ownerErr := f.Chown(UID, UID)
		if err := errors.Join(writeErr, syncErr, ownerErr, f.Close()); err != nil {
			_ = os.Remove(f.Name())
			return err
		}
		if err := os.Rename(f.Name(), filepath.Join(RuntimeDir, name)); err != nil {
			_ = os.Remove(f.Name())
			return err
		}
	}
	if s.Mode != "init" {
		if err := os.Remove(RuntimeDir + "/bootstrap.password"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Run executes only after a clean-environment exec as UID/GID 10001.
func Run(ctx context.Context, out io.Writer) error {
	var s Settings
	if err := mcpconfig.ReadPrivateJSON(RuntimeDir+"/startup.json", &s, true); err != nil {
		return err
	}
	if s.Mode == "maintenance" {
		return maintenance(ctx, s.Config, out)
	}
	if s.Mode == "init" {
		password, err := serverops.ReadPassword(RuntimeDir + "/bootstrap.password")
		if err != nil {
			return err
		}
		if err := cloud.Initialize(ctx, s.Config, s.Username, s.OwnerName, password, false); err != nil {
			return err
		}
		if err := os.Remove(RuntimeDir + "/bootstrap.password"); err != nil {
			return err
		}
		fmt.Fprintln(out, "Owner initialized. Set DING_MODE=serve and remove bootstrap variables before redeploying.")
	}
	return cloud.Run(ctx, s.Config, "railway", out)
}

func maintenanceHandler(c cloud.Config) http.Handler {
	origin, _ := url.Parse(c.PublicURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		probe := r.Method == "GET" && (r.URL.Path == "/readyz" || r.URL.Path == "/healthz")
		if r.Host != origin.Host && !(probe && r.Host == "healthcheck.railway.app") {
			w.WriteHeader(403)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != c.PublicURL {
			w.WriteHeader(403)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/readyz" {
			fmt.Fprintln(w, "maintenance")
			return
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(503)
		fmt.Fprintln(w, "Ding is in maintenance. Watch execution is stopped.")
	})
}

func maintenance(ctx context.Context, c cloud.Config, out io.Writer) error {
	srv := &http.Server{Addr: c.Listen, Handler: maintenanceHandler(c), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()
	fmt.Fprintln(out, "Maintenance mode: no stores opened and no watches or deliveries running.")
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(stop)
	}
}
