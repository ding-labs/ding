package transform

import (
	"context"
	"strings"
	"testing"
)

func TestStrictJSONRejectsAmbiguityAndBoundsWork(t *testing.T) {
	for _, raw := range []string{`{"a":1,"\u0061":2}`, `{"a":{"b":1,"b":2}}`, `{"x":1e999}`, `{"id":9007199254740993}`, `{} {}`, `[]`, "{\"x\":\"\xff\"}", `{"a":` + strings.Repeat("[", 33) + `0` + strings.Repeat("]", 33) + `}`} {
		if err := ValidateJSON(context.Background(), []byte(raw), 32, 1000); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if err := ValidateJSON(context.Background(), []byte(`{"id":"9007199254740993","value":1.25,"nested":{"ok":true}}`), 32, 1000); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(context.Background(), []byte(`{"a":[1,2,3]}`), 32, 3); err == nil {
		t.Fatal("node limit")
	}
}
func FuzzStrictJSON(f *testing.F) {
	for _, s := range []string{`{}`, `{"events":[{"a":1}]}`, `{"a":1,"a":2}`, `{"a":9007199254740993}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 65536 {
			t.Skip()
		}
		_ = ValidateJSON(context.Background(), []byte(s), 32, 1000)
	})
}
