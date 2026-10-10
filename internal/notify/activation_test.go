package notify

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestActivationRoutesWithoutAcceptingCommandsOrCredentials(t *testing.T) {
	target := Target{StateDir: filepath.Join(t.TempDir(), `space & $(command) "quote"`), WatchID: "api-health"}
	raw := ActivationURL(target)
	got, err := ParseActivation(raw)
	if err != nil || got != target {
		t.Fatal("target did not roundtrip", err)
	}
	home := Target{StateDir: target.StateDir}
	if got, err := ParseActivation(ActivationURL(home)); err != nil || got != home {
		t.Fatal("test notification did not route to Console home", err)
	}
	for _, bad := range []string{
		"https://example.test/watch", raw + "?token=secret", raw + "#secret", raw + "/",
		`ding-watch://watch/` + base64.RawURLEncoding.EncodeToString([]byte(`{"state":"relative","watch":"api"}`)),
		`ding-watch://watch/` + base64.RawURLEncoding.EncodeToString([]byte(`{"state":"/tmp","watch":"../../escape"}`)),
		`ding-watch://watch/` + base64.RawURLEncoding.EncodeToString([]byte(`{"state":"/tmp","watch":"api","command":"evil"}`)),
	} {
		if _, err := ParseActivation(bad); err == nil {
			t.Fatal("accepted untrusted activation", bad)
		}
	}
}
