package plan_test

import (
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/plan"
)

func TestHTTPDescriptionFollowsCompiledRule(t *testing.T) {
	manifest, _, err := onboarding.Manifest(onboarding.Request{URL: "https://example.com/health", Delivery: "console"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ every, span string }{{"30s", "1m0s"}, {"5m", "10m0s"}} {
		t.Run(tc.every, func(t *testing.T) {
			bundle, err := plan.Parse([]byte(strings.ReplaceAll(manifest, "every: 30s", "every: "+tc.every)))
			if err != nil {
				t.Fatal(err)
			}
			d := plan.Describe(bundle.Watches[0])
			for _, want := range []string{"every " + tc.every, "timeout 5s", "source errors separately"} {
				if !strings.Contains(d.Source, want) {
					t.Fatalf("source %q missing %q", d.Source, want)
				}
			}
			for _, want := range []string{"3 consecutive matches", "2 nonmatching inputs", "preserve the incident state", tc.span, "can add delay"} {
				if !strings.Contains(d.Policy, want) {
					t.Fatalf("policy %q missing %q", d.Policy, want)
				}
			}
			if !strings.Contains(d.Condition, "401 or 404 do not match") {
				t.Fatal(d.Condition)
			}
			definition := bundle.Watches[0].Definition
			definition.Spec.Condition.Operator = "ne"
			definition.Spec.Condition.Value = 200
			definition.Spec.Policy.Consecutive = 1
			compiled, err := plan.Compile(definition)
			if err != nil {
				t.Fatal(err)
			}
			d = plan.Describe(compiled)
			if d.Condition != "http.status does not equal 200." || strings.Contains(d.Policy, "3 matching checks") || strings.Contains(d.Condition, "do not match") {
				t.Fatal(d)
			}
		})
	}
}
