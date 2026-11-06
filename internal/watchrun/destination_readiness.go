package watchrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
)

type DestinationSetupStatus struct {
	store.DestinationReadiness
	AccountID      string `json:"accountID"`
	WorkspaceID    string `json:"workspaceID"`
	DestinationRef string `json:"destinationRef"`
	Revision       string `json:"revision,omitempty"`
	SetupURL       string `json:"setupURL"`
}

func interactiveDestinationPrincipal(p access.Principal) bool {
	return (p.Kind == "browser" || p.Kind == "personal") && p.Grant == "" && p.Allows(access.Destinations)
}

func (a *App) destinationMaterial(ctx context.Context, ref string) (destination plan.CompiledDestination, endpoint string, headers http.Header, err error) {
	err = a.Store.View(ctx, func(tx *store.Tx) error { var e error; destination, e = tx.Destination(ref, ""); return e })
	if err != nil {
		return
	}
	spec := destination.Definition.Spec
	headers = make(http.Header)
	if spec.Type == "console" || spec.Type == "desktop" {
		return
	}
	endpoint, err = source.Resolve(spec.URLRef, a.Lookup)
	if err != nil || source.URL(endpoint) != nil {
		err = fmt.Errorf("destination endpoint or credential is unavailable")
		return
	}
	for name, ref := range spec.Headers {
		value, e := source.Resolve(&ref, a.Lookup)
		if e != nil {
			err = fmt.Errorf("destination header credential is unavailable")
			return
		}
		headers.Set(name, value)
	}
	return
}
func destinationBinding(p access.Principal, d plan.CompiledDestination, endpoint string, headers http.Header) store.DestinationSetup {
	raw, _ := json.Marshal([]any{d, endpoint, headers})
	return store.DestinationSetup{Account: p.User, Workspace: p.Workspace, Destination: d.Definition.Metadata.ID, Revision: d.Revision, Digest: fmt.Sprintf("%x", sha256.Sum256(raw))}
}

func (a *App) DestinationSetupStatus(ctx context.Context, ref string) (DestinationSetupStatus, error) {
	p, ok := access.FromContext(ctx)
	if !ok || !p.Allows(access.Inspect) {
		return DestinationSetupStatus{}, access.ErrDenied
	}
	out := DestinationSetupStatus{AccountID: p.User, WorkspaceID: p.Workspace, DestinationRef: ref, DestinationReadiness: store.DestinationReadiness{State: "missing"}}
	q := url.Values{"account": {p.User}, "workspace": {p.Workspace}}
	if ref != "" {
		q.Set("destination", ref)
	}
	out.SetupURL = "/ui/destination-setup?" + q.Encode()
	if ref == "" {
		return out, nil
	}
	d, endpoint, headers, err := a.destinationMaterial(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		out.State = "unavailable"
		return out, nil
	}
	out.Revision = d.Revision
	binding := destinationBinding(p, d, endpoint, headers)
	err = a.Store.View(ctx, func(tx *store.Tx) error {
		var e error
		out.DestinationReadiness, e = tx.DestinationReadiness(binding, a.Now())
		return e
	})
	return out, err
}

func (a *App) ConfirmDestinationReceipt(ctx context.Context, request DestinationTestRequest) (store.DestinationReadiness, error) {
	p, ok := access.FromContext(ctx)
	if !ok || !interactiveDestinationPrincipal(p) || request.AccountID != p.User || request.WorkspaceID != p.Workspace {
		return store.DestinationReadiness{}, access.ErrDenied
	}
	d, endpoint, headers, err := a.destinationMaterial(ctx, request.DestinationRef)
	if err != nil {
		return store.DestinationReadiness{}, err
	}
	if d.Revision != request.Revision {
		return store.DestinationReadiness{}, store.ErrConflict
	}
	binding := destinationBinding(p, d, endpoint, headers)
	var out store.DestinationReadiness
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		current, e := tx.Destination(request.DestinationRef, "")
		if e != nil {
			return e
		}
		if current.Revision != d.Revision {
			return store.ErrConflict
		}
		out, e = tx.ConfirmDestinationProbe(request.OperationKey, binding, a.Now())
		return e
	})
	return out, err
}
