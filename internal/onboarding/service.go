package onboarding

import (
	"fmt"

	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type ServiceRequest struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	DestinationRef string `json:"destinationRef"`
	ExpectedStatus *int   `json:"expectedStatus,omitempty"`
}

// ServiceDefinition is the common Console/MCP recipe. It performs no source or
// delivery I/O; the caller must still apply its instance policy and review it.
func ServiceDefinition(r ServiceRequest, execution string) (watch.Definition, error) {
	if r.ID == "" || r.DestinationRef == "" {
		return watch.Definition{}, fmt.Errorf("watch ID and saved destination reference are required")
	}
	w := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: r.ID}, Spec: watch.Spec{
		Source:       watch.Source{Type: "http", URL: r.URL, Every: "30s", Timeout: "5s"},
		Condition:    watch.Condition{Field: "http.status", Operator: "gte", Value: 500},
		Policy:       watch.Policy{Trigger: "transition", Consecutive: 3, RecoverAfter: 2, OnUnknown: "hold-incident"},
		Destinations: []watch.Target{{Ref: r.DestinationRef, Events: []string{"firing", "recovered", "source_error", "source_recovered"}}},
	}}
	switch execution {
	case "local", "shared":
		w.Metadata.Name = r.ID
	case "cloud":
		w.Spec.Source.Every = "5m"
		w.Spec.Limits = watch.Limits{MaxBytes: integrationpolicy.CloudResponseBytes, MaxEntities: 10, MaxSamples: 100, MaxOutputs: 10}
	default:
		return watch.Definition{}, fmt.Errorf("unsupported execution target")
	}
	if r.ExpectedStatus != nil {
		if *r.ExpectedStatus < 100 || *r.ExpectedStatus > 599 {
			return watch.Definition{}, fmt.Errorf("expected HTTP status must be between 100 and 599")
		}
		w.Spec.Condition.Operator, w.Spec.Condition.Value = "ne", *r.ExpectedStatus
	}
	return w, nil
}

func ServiceManifest(r ServiceRequest, execution string) (string, error) {
	w, err := ServiceDefinition(r, execution)
	if err != nil {
		return "", err
	}
	raw, err := yaml.Marshal(w)
	if err != nil {
		return "", err
	}
	if _, err := plan.Parse(raw); err != nil {
		return "", err
	}
	return string(raw), nil
}
