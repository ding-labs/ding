package identity

import (
	"reflect"
	"testing"
)

func TestComponentBoundaries(t *testing.T) {
	cases := [][]string{{"a:b", "c"}, {"a", "b:c"}, {"a", ""}, {"a"}, {""}, {"\xff"}, {"\ufffd"}}
	seen := map[string]bool{}
	for _, parts := range cases {
		key := Key(parts...)
		if seen[key] {
			t.Fatalf("collision for %#v", parts)
		}
		seen[key] = true
		got, err := Parts(key)
		if err != nil || !reflect.DeepEqual(got, parts) {
			t.Fatalf("roundtrip %q: %#v %v", key, got, err)
		}
	}
	if _, err := Parts("broken"); err == nil {
		t.Fatal("accepted invalid key")
	}
}

func FuzzKeyRoundTrip(f *testing.F) {
	f.Add("a:b", "c=1,foo=x")
	f.Add("\xff", "世界\x00")
	f.Fuzz(func(t *testing.T, a, b string) {
		got, err := Parts(Key(a, b))
		if err != nil || len(got) != 2 || got[0] != a || got[1] != b {
			t.Fatalf("roundtrip failed: %q %q", a, b)
		}
		if Key(a, b) == Key(a+":"+b) {
			t.Fatal("component boundary collision")
		}
	})
}
