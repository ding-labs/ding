package plan

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watch"
)

const valid = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api}
spec:
  source: {type: http, url: 'https://example.com/health'}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {consecutive: 3, recoverAfter: 2}
  destinations: [{ref: ops}]
`

func TestCompileDefaultsAndIdentity(t *testing.T) {
	b, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	c := b.Watches[0]
	if c.Definition.Spec.Source.Every != "5s" || c.Definition.Spec.Policy.MaxGap != "10s" || c.Definition.Spec.Limits.MaxSamples != 10000 {
		t.Fatal(c)
	}
	d := c.Definition
	d.Metadata.Name = "Renamed"
	d.Spec.Message = "Changed text"
	d.Spec.Destinations = []watch.Target{{Ref: "elsewhere"}}
	changed, err := Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Fingerprint != c.Fingerprint || changed.Revision == c.Revision {
		t.Fatal("presentation/destination compatibility")
	}
	d.Spec.Source.Fields = map[string]string{"a": "a"}
	changed, _ = Compile(d)
	if changed.Fingerprint == c.Fingerprint {
		t.Fatal("source interpretation not fenced")
	}
	d = c.Definition
	d.Spec.GroupBy = []string{"z", "a"}
	before := append([]string{}, d.Spec.GroupBy...)
	first, err := Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Spec.GroupBy, before) {
		t.Fatal("compiler mutated caller")
	}
	d.Spec.GroupBy = []string{"a", "z"}
	second, _ := Compile(d)
	if first.Revision != second.Revision {
		t.Fatal("noncanonical revision")
	}
}
func TestStrictManifest(t *testing.T) {
	cases := []string{
		strings.Replace(valid, "v1alpha1", "v99", 1), strings.Replace(valid, "kind: Watch", "kind: Workflow", 1), strings.Replace(valid, "id: api", "id: ../path", 1),
		valid + "unexpected: true\n", valid + "---\n" + valid,
		strings.Replace(valid, "type: http", "type: browser", 1), strings.Replace(valid, "type: http", "type: http, typo: true", 1),
		strings.Replace(valid, "https://example.com/health", "https://token:secret@example.com", 1),
		strings.Replace(valid, "value: 500", "value: '500'", 1), strings.Replace(valid, "operator: gte", "operator: probably", 1),
		strings.Replace(valid, "consecutive: 3", "consecutive: -1", 1), strings.Replace(valid, "consecutive: 3", "onUnknown: pretend-healthy", 1),
		strings.Replace(valid, "ref: ops", "ref: ops, events: [imaginary]", 1),
		strings.Replace(valid, "policy: {", "groupBy: [a, a]\n  policy: {", 1),
		strings.Replace(valid, "type: http", "type: http, argv: [rm]", 1),
		strings.Replace(valid, "type: http", "type: http, every: -2s", 1),
		strings.Replace(valid, "type: http", "type: http, jq: 'this is not jq @'", 1),
		strings.Replace(valid, "field: http.status, operator: gte, value: 500", "numeric: 'avg(value) over run > 1'", 1),
		strings.Replace(valid, "field: http.status, operator: gte, value: 500", "missingFor: -1s", 1),
		"", "apiVersion: ding.ing/v1alpha1\nkind: Watch\nkind: Destination\n", "x: &a {}\ny: *a\n",
	}
	for i, input := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("accepted invalid manifest", input)
			}
		})
	}
}
func TestCapabilityShapes(t *testing.T) {
	b, _ := Parse([]byte(valid))
	d := b.Watches[0].Definition
	d.Spec.Source = watch.Source{Type: "command", Argv: []string{"status-tool", "--json"}, Directory: t.TempDir(), Env: map[string]watch.SecretRef{"TOKEN": {Env: "NO_LOOKUP"}}}
	if _, err := Compile(d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Source = watch.Source{Type: "push"}
	d.Spec.Condition = watch.Condition{MissingFor: "1m"}
	if _, err := Compile(d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Condition = watch.Condition{Field: "value", Numeric: "avg(value) over 5m > 8 AND value > 2"}
	if _, err := Compile(d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Condition = watch.Condition{Field: "enabled", Operator: "eq", Value: true}
	if _, err := Compile(d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Condition = watch.Condition{Field: "id", Operator: "new-event", DedupFor: "1h"}
	d.Spec.Policy = watch.Policy{Trigger: "level", Interval: "0s"}
	if _, err := Compile(d); err != nil {
		t.Fatal(err)
	}
}
func TestSecretReferencesNeverResolve(t *testing.T) {
	t.Setenv("WATCH_SECRET", "must-not-appear")
	input := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: ops}
spec: {type: webhook, urlRef: {env: WATCH_SECRET}}
`
	b, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(b)
	if strings.Contains(string(data), "must-not-appear") {
		t.Fatal("resolved a secret")
	}
	os.Unsetenv("WATCH_SECRET")
	next, err := Parse([]byte(input))
	if err != nil || next.Destinations[0].Revision != b.Destinations[0].Revision {
		t.Fatal("depends on credentials", err)
	}
}
func TestDestinationValidation(t *testing.T) {
	for _, spec := range []watch.DestinationSpec{{Type: "email"}, {Type: "webhook"}, {Type: "console", URLRef: &watch.SecretRef{Env: "X"}}, {Type: "slack", URLRef: &watch.SecretRef{Env: "bad-name"}}, {Type: "console", MaxAttempts: -1}, {Type: "console", MaxAge: "-1s"}, {Type: "console", InitialBackoff: "-1s"}} {
		if _, err := CompileDestination(watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: "d"}, Spec: spec}); err == nil {
			t.Fatal(spec)
		}
	}
}
func FuzzManifest(f *testing.F) {
	f.Add([]byte(valid))
	f.Add([]byte("bad"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		_, _ = Parse(data)
	})
}

func TestSchemaMatchesArtifact(t *testing.T) {
	generated, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile("../../schemas/watch-v1alpha1.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(saved)) != string(generated) {
		t.Fatal("regenerate schema with go run ./cmd/watch-schema")
	}
}
func TestExplicitNullAndInvalidWindows(t *testing.T) {
	for _, replacement := range []string{"field: field, operator: eq", "numeric: 'avg(value) over 0s > 1'", "field: value, operator: gt, value: 1, dedupFor: 1h"} {
		if _, err := Parse([]byte(strings.Replace(valid, "field: http.status, operator: gte, value: 500", replacement, 1))); err == nil {
			t.Fatal(replacement)
		}
	}
	input := strings.Replace(valid, "field: http.status, operator: gte, value: 500", "field: maybe, operator: eq, value: null", 1)
	b, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(b.Watches[0].Definition)
	if _, err := Parse(encoded); err != nil {
		t.Fatal("null lost in roundtrip", err)
	}
}
