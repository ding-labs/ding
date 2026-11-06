package cloud

import (
	"bytes"
	"fmt"

	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type FirstWatch struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	Destination    string `json:"destination"`
	Credential     string `json:"credential"`
	DestinationRef string `json:"destinationRef,omitempty"`
}

// Template makes the cloud's cadence and bounds explicit in the review. It does
// not rewrite or upload an existing local watch.
func Template(r FirstWatch) (string, error) {
	if r.DestinationRef != "" && (r.Destination != "" || r.Credential != "") {
		return "", fmt.Errorf("choose a new delivery type or an existing destination, not both")
	}
	destID := r.ID + "-alerts"
	if r.DestinationRef != "" {
		destID = r.DestinationRef
	}
	d := watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: r.ID + "-alerts"}, Spec: watch.DestinationSpec{Type: r.Destination, URLRef: &watch.SecretRef{Env: r.Credential}, InitialBackoff: "30s"}}
	w, err := onboarding.ServiceDefinition(onboarding.ServiceRequest{ID: r.ID, URL: r.URL, DestinationRef: destID}, "cloud")
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if r.DestinationRef == "" {
		if err := encoder.Encode(d); err != nil {
			return "", err
		}
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
