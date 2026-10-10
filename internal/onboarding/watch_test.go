package onboarding

import (
	"github.com/ding-labs/ding/internal/plan"
	"strings"
	"testing"
)

func TestFirstWatchUsesRealCompilerAndNoSecrets(t *testing.T) {
	m, desc, err := Manifest(Request{ID: "dev-api", URL: "http://127.0.0.1:3000/health", Delivery: "desktop"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := plan.Parse([]byte(m))
	if err != nil {
		t.Fatal(err)
	}
	w := b.Watches[0].Definition
	if len(desc) != 1 || w.Spec.Policy.Consecutive != 3 || w.Spec.Policy.RecoverAfter != 2 || b.Destinations[0].Definition.Spec.Type != "desktop" {
		t.Fatal(w)
	}
	if !strings.Contains(m, "source_error") {
		t.Fatal("missing unavailable-source alert")
	}
	for _, r := range []Request{{URL: "file:///private/data", Delivery: "desktop"}, {URL: "https://name:secret@example.com", Delivery: "desktop"}, {ID: "bad/id", URL: "https://example.com", Delivery: "desktop"}, {URL: "https://example.com", Delivery: "shell"}} {
		if _, _, err := Manifest(r); err == nil {
			t.Fatal("accepted unsafe first watch", r)
		}
	}
}
