// Package access defines permissions independently of hosting and login methods.
// Principals are installed by trusted authentication adapters, never HTTP headers.
package access

import (
	"context"
	"slices"
)

type Role string

const (
	Viewer        Role = "viewer"
	Editor        Role = "editor"
	Administrator Role = "administrator"
	Owner         Role = "owner"
)

type Action string

const (
	Inspect         Action = "inspect"
	Preview         Action = "preview"
	Manage          Action = "manage"
	Retry           Action = "retry"
	Destinations    Action = "destinations"
	Credentials     Action = "credentials"
	Members         Action = "members"
	Owners          Action = "owners"
	DeleteWorkspace Action = "delete_workspace"
	Host            Action = "host"
	Ingest          Action = "ingest"
)

var actions = []Action{Inspect, Preview, Manage, Retry, Destinations, Credentials, Members, Owners, DeleteWorkspace, Host, Ingest}

type Principal struct {
	User      string `json:"user"`
	Workspace string `json:"workspace"`
	Role      Role   `json:"role"`
	Kind      string `json:"kind"`
	Grant     string `json:"grant,omitempty"`
	Operator  bool   `json:"-"`
	// Nil scopes denote an interactive browser or host session. Delegated
	// credentials must supply explicit scopes, even when the list is empty.
	Scopes []string `json:"-"`
}

func (r Role) Valid() bool {
	return r == Viewer || r == Editor || r == Administrator || r == Owner
}

func (p Principal) Allows(action Action) bool {
	if p.User == "" || p.Workspace == "" || !slices.Contains(actions, action) {
		return false
	}
	if p.Operator {
		return true
	}
	if !p.Role.Valid() || action == Host {
		return false
	}
	if p.Kind == "device" || p.Kind == "integration" || p.Kind == "service" || p.Kind == "ingest" {
		if !slices.Contains(p.Scopes, string(action)) {
			return false
		}
	}
	switch action {
	case Inspect:
		return true
	case Preview, Manage, Retry:
		return p.Role != Viewer
	case Destinations, Credentials, Members:
		return p.Role == Administrator || p.Role == Owner
	case Owners, DeleteWorkspace:
		return p.Role == Owner
	case Ingest:
		return p.Kind == "ingest" || p.Kind == "service"
	}
	return false
}

func (p Principal) Capabilities() []string {
	out := []string{}
	for _, action := range actions {
		if p.Allows(action) {
			out = append(out, string(action))
		}
	}
	return out
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	p.Scopes = slices.Clone(p.Scopes)
	return context.WithValue(ctx, principalKey{}, p)
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
