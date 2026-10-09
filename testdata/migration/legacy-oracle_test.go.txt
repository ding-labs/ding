package migrate

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/ingester"
)

// Run before architectural deletion; the saved fixture remains afterward.
func TestLegacyOracleCapture(t *testing.T) {
	cases := parityCases(t)
	for i, c := range cases {
		cooldown, _ := time.ParseDuration(c.Cooldown)
		engine, err := evaluator.NewEngine([]evaluator.EngineRule{{Name: c.Name, Condition: c.Condition, Cooldown: cooldown, Message: c.Message, Match: c.Match}}, 10000)
		if err != nil {
			t.Fatal(err)
		}
		got := []parityAlert{}
		for n, input := range c.Inputs {
			raw, _ := json.Marshal(input.Data)
			events, err := ingester.ParseJSONLine(raw)
			if err != nil {
				t.Fatal(err)
			}
			now := parityStart.Add(time.Duration(input.Second) * time.Second)
			events[0].At = now
			alerts, err := engine.ProcessChecked(events[0], now)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range alerts {
				got = append(got, parityAlert{n, a.Message})
			}
		}
		if os.Getenv("DING_CAPTURE_LEGACY_ORACLE") == "1" {
			cases[i].Legacy = got
		} else if !reflect.DeepEqual(got, c.Legacy) {
			t.Fatalf("legacy drift %s: got %#v, want %#v", c.Name, got, c.Legacy)
		}
	}
	if os.Getenv("DING_CAPTURE_LEGACY_ORACLE") == "1" {
		raw, _ := json.MarshalIndent(cases, "", "  ")
		if err := os.WriteFile("../../testdata/migration/parity.json", append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
