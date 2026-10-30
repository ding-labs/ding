package notify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNotificationRejectsOversizedInputBeforeOSCall(t *testing.T) {
	for _, m := range []Message{{}, {ID: "test", Title: strings.Repeat("x", 513)}, {ID: "test", Body: strings.Repeat("x", 4097)}, {ID: "test", Body: "a\x00b"}, {ID: "test", Status: "unknown"}} {
		if err := Send(context.Background(), m); err == nil {
			t.Fatal("accepted invalid notification")
		}
	}
}

func TestNotificationContentIsPassedAsData(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DING_TEST_NOTIFICATION_PIPE", "1")
	m := Message{ID: "test", Title: "Ding", Body: `$HOME; $(touch /tmp/should-not-exist) <xml> "quotes"`}
	if err := helper(context.Background(), exe, []string{"-test.run=^TestNotificationPipeHelper$"}, m); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationPipeHelper(t *testing.T) {
	if code := os.Getenv("DING_TEST_NOTIFICATION_ERROR"); code != "" {
		if code != "crash" {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": code})
		}
		os.Exit(1)
	}
	if os.Getenv("DING_TEST_NOTIFICATION_PIPE") != "1" {
		return
	}
	var m Message
	if json.NewDecoder(os.Stdin).Decode(&m) != nil || m.Body != `$HOME; $(touch /tmp/should-not-exist) <xml> "quotes"` {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestNotificationFailureDistinguishesPermissionFromHelperFailure(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"notification_permission_denied", "notification_permission_pending", "crash"} {
		t.Run(code, func(t *testing.T) {
			t.Setenv("DING_TEST_NOTIFICATION_ERROR", code)
			err := helper(context.Background(), exe, []string{"-test.run=^TestNotificationPipeHelper$"}, Message{ID: "test"})
			failure, ok := err.(Failure)
			if code == "crash" {
				code = "notification_helper_failed"
			}
			if !ok || failure.Code != code {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNativeNotificationStatusDoesNotCrash(t *testing.T) {
	path := os.Getenv("DING_TEST_NOTIFICATION_HELPER")
	if path == "" {
		t.Skip("set DING_TEST_NOTIFICATION_HELPER to qualify a native macOS bundle")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--status").Output()
	if err != nil {
		t.Fatal(err)
	}
	var s Settings
	if json.Unmarshal(out, &s) != nil || s.Authorization == "" {
		t.Fatalf("invalid native settings: %s", out)
	}
}
