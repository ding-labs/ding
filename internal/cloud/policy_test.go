package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

const fixture = `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: alerts}
spec: {type: webhook, urlRef: {env: WEBHOOK}}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: health}
spec:
  source: {type: http, url: 'https://example.com/health', every: 5m, timeout: 5s}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {trigger: transition, consecutive: 3, recoverAfter: 2}
  limits: {maxBytes: 65536, maxEntities: 10, maxSamples: 100, maxOutputs: 10}
  destinations: [{ref: alerts, events: [firing, recovered]}]
`

func TestCloudPolicyAndRuntimeMutationBoundary(t *testing.T) {
	b, err := plan.Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := Policy(b); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := watchrun.New(db)
	a.ValidateBundle = Policy
	a.Limits = Limits()
	for _, manifest := range []string{
		strings.ReplaceAll(fixture, "every: 5m", "every: 1s"),
		strings.ReplaceAll(fixture, "https://example.com/health", "http://169.254.169.254/latest/meta-data"),
		strings.ReplaceAll(fixture, "maxOutputs: 10", "maxOutputs: 100"),
		strings.ReplaceAll(fixture, "type: webhook, urlRef: {env: WEBHOOK}", "type: desktop"),
	} {
		for _, dry := range []bool{false, true} {
			if _, err := a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: manifest, DryRun: dry}); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		}
	}
	list, err := a.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatal("rejected policy changed state")
	}
	if _, err := a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: fixture}); err != nil {
		t.Fatal(err)
	}
}
