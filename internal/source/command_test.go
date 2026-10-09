package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

func TestCommandHelper(t *testing.T) {
	mode := os.Getenv("DING_SOURCE_MODE")
	if mode == "" {
		return
	}
	switch mode {
	case "json":
		wd, _ := os.Getwd()
		json.NewEncoder(os.Stdout).Encode(map[string]any{"directory": wd, "leak": os.Getenv("DING_PARENT_SECRET"), "arg": os.Args[len(os.Args)-1]})
		os.Exit(0)
	case "large":
		fmt.Print(strings.Repeat("x", 4096))
		os.Exit(0)
	case "error":
		fmt.Fprintln(os.Stderr, "private credential that must not be retained")
		os.Exit(7)
	case "wait":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "grandchild":
		marker := os.Getenv("DING_MARKER")
		os.WriteFile(marker+".ready", []byte("ready"), 0600)
		time.Sleep(3 * time.Second)
		os.WriteFile(marker, []byte("escaped"), 0600)
		os.Exit(0)
	case "parent", "parent-exit":
		child := exec.Command(os.Args[0], "-test.run=^TestCommandHelper$")
		child.Env = []string{"DING_SOURCE_MODE=grandchild", "DING_MARKER=" + os.Getenv("DING_MARKER"), "GORACE=atexit_sleep_ms=0"}
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("DING_MARKER") + ".ready"); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if mode == "parent-exit" {
			fmt.Println(`{"ok":true}`)
			os.Exit(0)
		}
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	os.Exit(2)
}
func commandPlan(t *testing.T, mode string) (plan.Compiled, Command) {
	t.Helper()
	d := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "command"}, Spec: watch.Spec{Source: watch.Source{Type: "command", Argv: []string{os.Args[0], "-test.run=^TestCommandHelper$", "--", "literal; $(touch not-a-command)"}, Directory: t.TempDir(), Timeout: "2s", Env: map[string]watch.SecretRef{"DING_SOURCE_MODE": {Env: "MODE"}, "GORACE": {Env: "RACE"}}}, Condition: watch.Condition{Field: "ok", Operator: "eq", Value: true}}}
	p, err := plan.Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	return p, Command{Lookup: func(name string) (string, bool) {
		if name == "MODE" {
			return mode, true
		}
		if name == "RACE" {
			return "atexit_sleep_ms=0", true
		}
		return "", false
	}}
}
func TestCommandArgvDirectoryAndEnvironment(t *testing.T) {
	t.Setenv("DING_PARENT_SECRET", "secret")
	p, c := commandPlan(t, "json")
	b := c.Fetch(context.Background(), p, "", time.Now())
	if b.Observations[0].Health != "ok" {
		t.Fatal(b)
	}
	fields := b.Observations[0].Fields
	actualDir, ok := fields["directory"].(string)
	if !ok {
		t.Fatal(fields)
	}
	expectedInfo, err := os.Stat(p.Definition.Spec.Source.Directory)
	if err != nil {
		t.Fatal(err)
	}
	actualInfo, err := os.Stat(actualDir)
	if err != nil {
		t.Fatal(err)
	}
	if fields["leak"] != "" || !os.SameFile(expectedInfo, actualInfo) || fields["arg"] != "literal; $(touch not-a-command)" {
		t.Fatal(fields)
	}

}
func TestCommandFailureLimits(t *testing.T) {
	for _, tc := range []struct{ mode, reason string }{{"large", "command_output_limit"}, {"error", "command_failed"}, {"wait", "command_timeout"}} {
		t.Run(tc.mode, func(t *testing.T) {
			p, c := commandPlan(t, tc.mode)
			p.Definition.Spec.Limits.MaxBytes = 1024
			if tc.mode == "wait" {
				p.Definition.Spec.Source.Timeout = "200ms"
			}
			b := c.Fetch(context.Background(), p, "", time.Now())
			if b.Observations[0].Health != "unknown" || b.Observations[0].Detail != tc.reason || b.Observations[0].Fields != nil {
				t.Fatal(b)
			}
		})
	}
	p, c := commandPlan(t, "json")
	c.Lookup = func(string) (string, bool) { return "", false }
	if b := c.Fetch(context.Background(), p, "", time.Now()); b.Observations[0].Detail != "missing_credentials" {
		t.Fatal(b)
	}
	p, c = commandPlan(t, "json")
	p.Definition.Spec.Source.Argv[0] = filepath.Join(t.TempDir(), "absent")
	if b := c.Fetch(context.Background(), p, "", time.Now()); b.Observations[0].Detail != "command_start_failed" {
		t.Fatal(b)
	}
}
func TestCommandTerminatesDescendants(t *testing.T) {
	for _, mode := range []string{"parent", "parent-exit"} {
		t.Run(mode, func(t *testing.T) {
			p, c := commandPlan(t, mode)
			marker := filepath.Join(t.TempDir(), "child-ran")
			p.Definition.Spec.Source.Env["DING_MARKER"] = watch.SecretRef{Env: "MARKER"}
			lookup := c.Lookup
			c.Lookup = func(name string) (string, bool) {
				if name == "MARKER" {
					return marker, true
				}
				return lookup(name)
			}
			p.Definition.Spec.Source.Timeout = "2s"
			before := time.Now()
			b := c.Fetch(context.Background(), p, "", before)
			if b.Observations[0].Health != "unknown" || time.Since(before) > 4*time.Second {
				t.Fatal("command was not bounded", b)
			}
			if _, err := os.Stat(marker + ".ready"); err != nil {
				t.Fatal("descendant was not started", err)
			}
			time.Sleep(3200 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("descendant survived", err)
			}
		})
	}
}
