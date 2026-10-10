package service_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/service"
	"github.com/ding-labs/ding/internal/watchrun"
)

// Explicit opt-in: normal go test never installs an operating-system service.
func TestNativeServiceLifecycle(t *testing.T) {
	if os.Getenv("DING_TEST_NATIVE_SERVICE") != "1" {
		t.Skip("native service test requires explicit opt-in")
	}
	bin := os.Getenv("DING_TEST_BINARY")
	if bin == "" {
		t.Fatal("set DING_TEST_BINARY to a built ding executable")
	}
	dir := t.TempDir()
	r, err := install.Inspect(bin, dir, "dev", "standalone")
	if err != nil {
		t.Fatal(err)
	}
	if err := install.Create(r); err != nil {
		t.Fatal(err)
	}
	m, err := service.New(r)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := m.Install(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.Uninstall(ctx); err != nil {
			t.Errorf("native service cleanup: %v", err)
		}
	})
	ready := func() control.Client {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			c, e := control.Connect(dir)
			if e != nil {
				continue
			}
			c.HTTP.Timeout = time.Second
			raw, e := c.Call(ctx, "GET", "/v1/doctor", nil)
			if e != nil {
				continue
			}
			var health watchrun.Doctor
			if json.Unmarshal(raw, &health) == nil && health.Running && !health.Closing {
				return c
			}
		}
		t.Fatal("native daemon did not become ready")
		return control.Client{}
	}
	if err := m.Action(ctx, "start"); err != nil {
		t.Fatal(err)
	}
	c := ready()
	manifest, _, err := onboarding.Manifest(onboarding.Request{ID: "restart-proof", URL: "http://127.0.0.1:1/health", Delivery: "console"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Call(ctx, "POST", "/v1/apply", watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err = m.Action(ctx, "restart"); err != nil {
		t.Fatal(err)
	}
	c = ready()
	raw, err := c.Call(ctx, "GET", "/v1/watches", nil)
	if err != nil {
		t.Fatal(err)
	}
	var watches []any
	if json.Unmarshal(raw, &watches) != nil || len(watches) != 1 {
		t.Fatal("restart lost watch state")
	}
	if err = m.Action(ctx, "stop"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if _, err = c.Call(ctx, "GET", "/v1/doctor", nil); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stop left daemon running")
		}
	}
	if err = m.Action(ctx, "start"); err != nil {
		t.Fatal(err)
	}
	_ = ready()
}
