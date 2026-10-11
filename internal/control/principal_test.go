package control

import (
	"context"
	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http/httptest"
	"testing"
)

func TestPersonalPrincipalIsStableAndNeverReplacesSharedActor(t *testing.T) {
	db, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := watchrun.New(db)
	r, err := personalRequest(httptest.NewRequest("POST", "/v1/apply", nil), app)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := access.FromContext(r.Context())
	if !ok || !p.Operator || p.User == "" {
		t.Fatal(p)
	}
	again, err := personalRequest(httptest.NewRequest("POST", "/v1/apply", nil), app)
	if err != nil {
		t.Fatal(err)
	}
	same, _ := access.FromContext(again.Context())
	if p.User != same.User {
		t.Fatal("unstable owner")
	}
	viewer := access.Principal{User: "reader", Workspace: "team", Role: access.Viewer, Kind: "browser"}
	r = r.WithContext(access.WithPrincipal(r.Context(), viewer))
	r, err = personalRequest(r, app)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = access.FromContext(r.Context())
	if p.User != "reader" || p.Operator {
		t.Fatal("bridge elevated viewer")
	}
}
