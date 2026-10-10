// Package onboarding builds account-free templates through the watch compiler.
package onboarding

import (
	"bytes"
	"fmt"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type Request struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Delivery string `json:"delivery"`
}

func Manifest(r Request) (string, []plan.Description, error) {
	if r.Delivery != "desktop" && r.Delivery != "console" {
		return "", nil, fmt.Errorf("choose desktop or console delivery; configure network destinations in Workbench")
	}
	if r.ID == "" {
		r.ID = "first-watch"
	}
	destID := r.ID + "-alerts"
	d := watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: destID}, Spec: watch.DestinationSpec{Type: r.Delivery}}
	w := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: r.ID, Name: r.ID}, Spec: watch.Spec{
		Source:       watch.Source{Type: "http", URL: r.URL, Every: "30s", Timeout: "5s"},
		Condition:    watch.Condition{Field: "http.status", Operator: "gte", Value: 500},
		Policy:       watch.Policy{Trigger: "transition", Consecutive: 3, RecoverAfter: 2, OnUnknown: "hold-incident"},
		Destinations: []watch.Target{{Ref: destID, Events: []string{"firing", "recovered", "source_error", "source_recovered"}}},
	}}
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(d); err != nil {
		return "", nil, err
	}
	if err := e.Encode(w); err != nil {
		return "", nil, err
	}
	if err := e.Close(); err != nil {
		return "", nil, err
	}
	bundle, err := plan.Parse(b.Bytes())
	if err != nil {
		return "", nil, err
	}
	description := []plan.Description{plan.Describe(bundle.Watches[0])}
	return b.String(), description, nil
}
