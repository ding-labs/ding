// Package access defines permissions independently of hosting and login methods.
// Principals are installed by trusted authentication adapters, never HTTP headers.
package access

import (
	"context"
	"errors"
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
	Move            Action = "move"
	DeleteWorkspace Action = "delete_workspace"
	Host            Action = "host"
	Ingest          Action = "ingest"
)

var actions = []Action{Inspect, Preview, Manage, Retry, Destinations, Credentials, Members, Owners, Move, DeleteWorkspace, Host, Ingest}

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
		scope := string(action)
		// Existing CLI/service owner delegation retains its handoff authority.
		// Integration grants must opt in to the separate move scope.
		if action == Move && p.Kind != "integration" && slices.Contains(p.Scopes, "owners") {
			scope = "owners"
		}
		if p.Kind == "integration" && action == Destinations {
			scope = "manage"
		}
		if !slices.Contains(p.Scopes, scope) {
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
	case Owners, Move, DeleteWorkspace:
		return p.Role == Owner
	case Ingest:
		return p.Kind == "ingest" || p.Kind == "service"
	}
	return false
}

var ErrDenied = errors.New("workspace permission denied")

func Require(ctx context.Context, action Action) error {
	if p, ok := FromContext(ctx); ok && !p.Allows(action) {
		return ErrDenied
	}
	return nil
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

type operationKey struct{}

// WithOperation records only a fixed action and resource path; never request
// bodies, headers, query strings, passwords, or credential values.
func WithOperation(ctx context.Context, action, target string) context.Context {
	if len(action) > 64 {
		action = action[:64]
	}
	if len(target) > 512 {
		target = target[:512]
	}
	return context.WithValue(ctx, operationKey{}, [2]string{action, target})
}
func Operation(ctx context.Context) (string, string) {
	v, _ := ctx.Value(operationKey{}).([2]string)
	return v[0], v[1]
}
