package onboarding_test

import (
	"reflect"
	"testing"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/plan"
)

func TestServiceRecipeMatchesConsoleAndSupportsExpectedStatus(t *testing.T) {
	request := onboarding.ServiceRequest{ID: "service", URL: "https://example.com/health", DestinationRef: "alerts"}
	local, _, err := onboarding.Manifest(onboarding.Request{ID: request.ID, URL: request.URL, DestinationRef: request.DestinationRef})
	if err != nil {
		t.Fatal(err)
	}
	hosted, err := cloud.Template(cloud.FirstWatch{ID: request.ID, URL: request.URL, DestinationRef: request.DestinationRef})
	if err != nil {
		t.Fatal(err)
	}
	for mode, console := range map[string]string{"local": local, "shared": local, "cloud": hosted} {
		t.Run(mode, func(t *testing.T) {
			manifest, err := onboarding.ServiceManifest(request, mode)
			if err != nil {
				t.Fatal(err)
			}
			got, err := plan.Parse([]byte(manifest))
			if err != nil {
				t.Fatal(err)
			}
			want, err := plan.Parse([]byte(console))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || len(got.Destinations) != 0 {
				t.Fatal("starter drift or destination redefinition")
			}
			r := request
			r.ExpectedStatus = new(204)
			manifest, err = onboarding.ServiceManifest(r, mode)
			if err != nil {
				t.Fatal(err)
			}
			got, err = plan.Parse([]byte(manifest))
			if err != nil {
				t.Fatal(err)
			}
			condition := got.Watches[0].Definition.Spec.Condition
			if condition.Operator != "ne" || condition.Value != float64(204) {
				t.Fatal("expected-status rule lost", condition)
			}
		})
	}
	for _, value := range []int{0, 99, 600} {
		r := request
		r.ExpectedStatus = &value
		if _, err := onboarding.ServiceManifest(r, "cloud"); err == nil {
			t.Fatal("accepted invalid status", value)
		}
	}
	for _, r := range []onboarding.ServiceRequest{{URL: request.URL, DestinationRef: "alerts"}, {ID: "service", URL: request.URL}, {ID: "bad/id", URL: request.URL, DestinationRef: "alerts"}} {
		if _, err := onboarding.ServiceManifest(r, "cloud"); err == nil {
			t.Fatal("accepted invalid request", r)
		}
	}
}
