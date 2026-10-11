package access

import "testing"

func TestRoleAndDelegationBoundaries(t *testing.T) {
	for _, test := range []struct {
		role   Role
		kind   string
		scopes []string
		action Action
		allow  bool
	}{
		{Viewer, "browser", nil, Inspect, true}, {Viewer, "browser", nil, Manage, false},
		{Editor, "browser", nil, Manage, true}, {Editor, "browser", nil, Credentials, false},
		{Administrator, "browser", nil, Members, true}, {Administrator, "browser", nil, Owners, false},
		{Owner, "browser", nil, DeleteWorkspace, true}, {Owner, "browser", nil, Host, false},
		{Owner, "device", nil, Manage, false}, {Owner, "device", []string{"inspect"}, Manage, false},
		{Editor, "integration", []string{"manage"}, Manage, true}, {Viewer, "integration", []string{"manage"}, Manage, false},
		{Owner, "browser", nil, Ingest, false}, {Viewer, "ingest", []string{"ingest"}, Ingest, true},
		{Owner, "browser", nil, Action("future"), false},
	} {
		p := Principal{User: "u", Workspace: "w", Role: test.role, Kind: test.kind, Scopes: test.scopes}
		if got := p.Allows(test.action); got != test.allow {
			t.Errorf("%+v: got %v", test, got)
		}
	}
}

func TestRouteDefaultDenial(t *testing.T) {
	for _, test := range []struct {
		method, path string
		action       Action
	}{
		{"GET", "/v1/watches/health", Inspect}, {"POST", "/v1/watches/health/lifecycle", Manage},
		{"GET", "/v1/console/backup/abc", Host}, {"POST", "/v1/integrations/preview", Preview},
	} {
		if a, ok := Route(test.method, test.path); !ok || a != test.action {
			t.Errorf("%+v: %q %v", test, a, ok)
		}
	}
	for _, path := range []string{"/v1/new-admin-operation", "/v1/watches/a/b", "/v1/integrations/unsafe"} {
		if _, ok := Route("POST", path); ok {
			t.Errorf("unmapped operation allowed: %s", path)
		}
	}
}
