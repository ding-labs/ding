package control

import (
	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http"
)

// Only call after the personal credential boundary. Shared adapters already
// supplied a principal and must never acquire host authority here.
func personalRequest(r *http.Request, app *watchrun.App) (*http.Request, error) {
	if _, ok := access.FromContext(r.Context()); ok {
		return r, nil
	}
	var workspace string
	if err := app.Store.View(r.Context(), func(tx *store.Tx) error { var err error; workspace, err = tx.Identity(); return err }); err != nil {
		return r, err
	}
	p := access.Principal{User: "local-owner:" + workspace, Workspace: workspace, Role: access.Owner, Kind: "personal", Operator: true}
	return r.WithContext(access.WithOperation(access.WithPrincipal(r.Context(), p), r.Method, r.URL.Path)), nil
}
