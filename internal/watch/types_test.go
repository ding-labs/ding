package watch

import (
	"encoding/json"
	"math"
	"testing"
)

func TestStructuredEntityIdentity(t *testing.T) {
	seen := map[string]bool{}
	for _, fields := range []map[string]any{{}, {"a": nil}, {"a": ""}, {"a": false}, {"a": 0.0}, {"a": "0"}, {"a": "x,b=y", "b": ""}, {"a": "x", "b": "y,b="}} {
		key, err := EntityKey([]string{"a", "b"}, fields)
		if err != nil || seen[key] {
			t.Fatal("collision", key, err)
		}
		seen[key] = true
	}
	a, _ := EntityKey([]string{"a"}, map[string]any{"a": "same", "noise": "one"})
	b, _ := EntityKey([]string{"a"}, map[string]any{"a": "same", "noise": "two"})
	if a != b {
		t.Fatal("incidental field changes entity")
	}
}
func TestVersionedEnvelope(t *testing.T) {
	b, err := json.Marshal(Envelope{APIVersion: APIVersion, Error: &Error{Code: "invalid_manifest", Message: "invalid"}})
	if err != nil || string(b) != `{"apiVersion":"ding.ing/v1alpha1","error":{"code":"invalid_manifest","message":"invalid"}}` {
		t.Fatal(string(b), err)
	}
}

func TestCanonicalRevision(t *testing.T) {
	a, err := Revision(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Revision(map[string]any{"a": 1, "b": 2})
	if a != b {
		t.Fatal("map ordering")
	}
	if _, err := Revision(math.Inf(1)); err == nil {
		t.Fatal("non-finite revision")
	}
	for _, v := range []any{map[string]any{"nested": true}, []any{1}, string([]byte{255}), math.Inf(1)} {
		if _, err := EntityKey([]string{"a"}, map[string]any{"a": v}); err == nil {
			t.Fatal(v)
		}
	}
}
