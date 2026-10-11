package watchrun

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

func TestEditorCannotRedirectCredentialsOrChangeDestinations(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	manifest := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: console}
spec: {type: console}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: health}
spec:
  source: {type: http, url: 'https://internal.example/health', every: 1m, headers: {Authorization: {env: TOKEN}}}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {trigger: transition}
  destinations: [{ref: console, events: [firing]}]
`
	owner := access.WithPrincipal(ctx, access.Principal{User: "a", Workspace: "w", Role: access.Owner, Kind: "browser"})
	editor := access.WithPrincipal(ctx, access.Principal{User: "b", Workspace: "w", Role: access.Editor, Kind: "browser"})
	if _, err := a.Apply(owner, ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	allowed := strings.ReplaceAll(manifest, "value: 500", "value: 400")
	if _, err := a.Apply(editor, ApplyRequest{Manifest: allowed}); err != nil {
		t.Fatal("ordinary condition edit rejected", err)
	}
	for _, bad := range []string{strings.ReplaceAll(manifest, "internal.example", "attacker.example"), strings.ReplaceAll(manifest, "type: console", "type: desktop")} {
		for _, dry := range []bool{true, false} {
			if _, err := a.Apply(editor, ApplyRequest{Manifest: bad, DryRun: dry}); !errors.Is(err, access.ErrDenied) {
				t.Fatal("unapproved resource use", err)
			}
		}
	}
	viewer := access.WithPrincipal(ctx, access.Principal{User: "c", Workspace: "w", Role: access.Viewer, Kind: "browser"})
	if _, err := a.Lifecycle(viewer, "health", LifecycleRequest{Action: "pause"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("viewer changed runtime", err)
	}
}
