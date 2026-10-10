package cloud

import (
	"bytes"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type FirstWatch struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Destination string `json:"destination"`
	Credential  string `json:"credential"`
}

// Template makes the cloud's cadence and bounds explicit in the review. It does
// not rewrite or upload an existing local watch.
func Template(r FirstWatch) (string, error) {
	d := watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: r.ID + "-alerts"}, Spec: watch.DestinationSpec{Type: r.Destination, URLRef: &watch.SecretRef{Env: r.Credential}, InitialBackoff: "30s"}}
	w := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: r.ID}, Spec: watch.Spec{
		Source:       watch.Source{Type: "http", URL: r.URL, Every: "5m", Timeout: "5s"},
		Condition:    watch.Condition{Field: "http.status", Operator: "gte", Value: 500},
		Policy:       watch.Policy{Trigger: "transition", Consecutive: 3, RecoverAfter: 2, OnUnknown: "hold-incident"},
		Limits:       watch.Limits{MaxBytes: 64 << 10, MaxEntities: 10, MaxSamples: 100, MaxOutputs: 10},
		Destinations: []watch.Target{{Ref: d.Metadata.ID, Events: []string{"firing", "recovered", "source_error", "source_recovered"}}},
	}}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(d); err != nil {
		return "", err
	}
	if err := encoder.Encode(w); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	bundle, err := plan.Parse(out.Bytes())
	if err != nil {
		return "", err
	}
	if err := Policy(bundle); err != nil {
		return "", err
	}
	return out.String(), nil
}
