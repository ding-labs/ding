package migrate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type parityInput struct {
	Second int            `json:"second"`
	Data   map[string]any `json:"data"`
}
type parityAlert struct {
	Input   int    `json:"input"`
	Message string `json:"message"`
}
type parityCase struct {
	Name      string            `json:"name"`
	Condition string            `json:"condition"`
	Cooldown  string            `json:"cooldown,omitempty"`
	Message   string            `json:"message"`
	Match     map[string]string `json:"match,omitempty"`
	Inputs    []parityInput     `json:"inputs"`
	Legacy    []parityAlert     `json:"legacy"`
	Changed   bool              `json:"changed,omitempty"`
	Watch     []parityAlert     `json:"watch,omitempty"`
}

var parityStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func parityCases(t *testing.T) []parityCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "migration", "parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []parityCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}
func TestCapturedLegacyParity(t *testing.T) {
	for _, c := range parityCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			cfg := legacyConfig{Rules: []legacyRule{{Name: c.Name, Condition: c.Condition, Cooldown: c.Cooldown, Message: c.Message, Match: c.Match}}}
			raw, _ := yaml.Marshal(cfg)
			result, err := Convert(raw)
			if err != nil || result.Report.Converted != 1 {
				t.Fatal(result.Report, err)
			}
			bundle, err := plan.Parse(result.Files[0].Content)
			if err != nil {
				t.Fatal(err)
			}
			p := bundle.Watches[0]
			e, err := condition.New(p.Definition, p.Revision)
			if err != nil {
				t.Fatal(err)
			}
			states := map[string]condition.State{}
			got := []parityAlert{}
			for n, input := range c.Inputs {
				raw, _ := json.Marshal(input.Data)
				fields, err := transform.Project(context.Background(), raw, p.Definition.Spec.Source.JQ, nil, 100, 1<<20)
				if err != nil || len(fields) != 1 {
					t.Fatal(fields, err)
				}
				now := parityStart.Add(time.Duration(input.Second) * time.Second)
				o := watch.Observation{Sequence: int64(n + 1), AcceptedAt: now, Health: "ok", Fields: fields[0]}
				keys, _, err := e.Route(nil, o)
				if err != nil {
					t.Fatal(err)
				}
				for _, key := range keys {
					s := states[key]
					s.Entity = key
					r, err := e.Evaluate(s, o, now)
					if err != nil {
						t.Fatal(err)
					}
					states[key] = r.State
					for _, event := range r.Events {
						if event.Type == "firing" {
							got = append(got, parityAlert{n, event.Message})
						}
					}
				}
			}
			want := c.Legacy
			if c.Changed {
				want = c.Watch
				if want == nil {
					want = []parityAlert{}
				}
				if len(result.Report.Rules[0].Warnings) == 0 {
					t.Fatal("undeclared semantic change")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}
