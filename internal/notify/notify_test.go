package notify

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNotificationRejectsOversizedInputBeforeOSCall(t *testing.T) {
	for _, m := range []Message{{}, {ID: "test", Title: strings.Repeat("x", 513)}, {ID: "test", Body: strings.Repeat("x", 4097)}, {ID: "test", Body: "a\x00b"}} {
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
	if os.Getenv("DING_TEST_NOTIFICATION_PIPE") != "1" {
		return
	}
	var m Message
	if json.NewDecoder(os.Stdin).Decode(&m) != nil || m.Body != `$HOME; $(touch /tmp/should-not-exist) <xml> "quotes"` {
		os.Exit(1)
	}
	os.Exit(0)
}
