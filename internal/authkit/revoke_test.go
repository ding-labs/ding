package authkit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRevokeExactClientAcrossPages(t *testing.T) {
	var deleted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-key" {
			t.Error("missing API authentication")
		}
		if !strings.HasPrefix(r.URL.Path, "/user_management/users/user_owner/authorized_applications") {
			t.Error("wrong subject")
		}
		if r.Method == "DELETE" {
			deleted = r.URL.Path
			w.WriteHeader(204)
			return
		}
		if r.URL.Query().Get("after") == "" {
			fmt.Fprint(w, `{"object":"list","data":[{"application":{"id":"app_public","client_id":"public"}}],"list_metadata":{"after":"next"}}`)
		} else {
			fmt.Fprint(w, `{"object":"list","data":[{"id":"authorization_not_application","application":{"id":"app_private","client_id":"private"}}],"list_metadata":{"after":null}}`)
		}
	}))
	defer server.Close()
	c := NewRevoker("private-key")
	c.base = server.URL
	if err := c.Revoke(context.Background(), "user_owner", "private"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/user_management/users/user_owner/authorized_applications/app_private" {
		t.Fatal("wrong application revoked", deleted)
	}
}

func TestRevokeFailuresAndIdempotence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"absent", `{"object":"list","data":[],"list_metadata":{"after":null}}`, 200, false},
		{"bad-body", `{"object":"list"}`, 200, true},
		{"invalid-id", `{"object":"list","data":[{"application":{"id":"../other","client_id":"private"}}]}`, 200, true},
		{"loop", `{"object":"list","data":[],"list_metadata":{"after":"same"}}`, 200, true},
		{"unavailable", `private-key`, 503, true},
		{"unauthorized", `private-key`, 401, true},
		{"redirect", `private-key`, 302, true},
		{"already-deleted", `{"object":"list","data":[{"application":{"id":"app_private","client_id":"private"}}]}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					w.WriteHeader(404)
					return
				}
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c := NewRevoker("private-key")
			c.base = server.URL
			err := c.Revoke(context.Background(), "user_owner", "private")
			if (err != nil) != tc.wantError {
				t.Fatal(err)
			}
			if err != nil && strings.Contains(err.Error(), "private-key") {
				t.Fatal("credential leaked")
			}
		})
	}
}
