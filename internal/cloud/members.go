package cloud

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

func (s *Server) membersAPI(w http.ResponseWriter, r *http.Request, p access.Principal) bool {
	path := r.URL.Path
	if path == "/v1/audit" && r.Method == "GET" {
		before, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		if r.URL.Query().Get("before") == "" {
			before, err = 0, nil
		}
		if err != nil || before < 0 {
			cloudFail(w, 400, "invalid_cursor", "Invalid audit cursor.")
			return true
		}
		if r.URL.Query().Get("source") == "runtime" {
			t, err := s.Pool.Get(r.Context(), p.Workspace)
			var out []store.Audit
			if err == nil {
				err = t.App.Store.View(r.Context(), func(tx *store.Tx) error { var e error; out, e = tx.Audit(before); return e })
			}
			if err != nil {
				cloudFail(w, 503, "audit_unavailable", "Audit unavailable.")
			} else {
				cloudWrite(w, 200, out)
			}
			return true
		}
		out, err := s.DB.Audit(r.Context(), p.Workspace, before)
		if err != nil {
			cloudFail(w, 503, "audit_unavailable", "Audit unavailable.")
		} else {
			cloudWrite(w, 200, out)
		}
		return true
	}
	if path != "/v1/members" && !strings.HasPrefix(path, "/v1/members/") {
		return false
	}
	if path == "/v1/members" && r.Method == "GET" {
		out, err := s.DB.Members(r.Context(), p.Workspace)
		if err != nil {
			cloudFail(w, 503, "members_unavailable", "Members unavailable.")
		} else {
			cloudWrite(w, 200, out)
		}
		return true
	}
	var input struct {
		User     string      `json:"user"`
		Username string      `json:"username"`
		Name     string      `json:"name"`
		Role     access.Role `json:"role"`
		Disabled *bool       `json:"disabled"`
		Reset    bool        `json:"reset"`
	}
	if r.Method != "DELETE" && decodeCloud(r, &input) != nil {
		cloudFail(w, 400, "invalid_request", "Provide member details.")
		return true
	}
	if path == "/v1/members" && r.Method == "POST" && input.User == "" {
		if !s.passwordsEnabled() {
			cloudFail(w, 400, "identity_required", "Add an existing verified user by ID.")
			return true
		}
		out, err := s.DB.Invite(r.Context(), p, input.Username, input.Name, input.Role)
		if err != nil {
			cloudFail(w, 400, "invite_failed", err.Error())
		} else {
			cloudWrite(w, 201, map[string]any{"user": out.User, "url": s.PublicURL + "/ui/#activate=" + out.Token, "expiresAt": out.ExpiresAt})
		}
		return true
	}
	user := input.User
	if strings.HasPrefix(path, "/v1/members/") {
		user = strings.TrimPrefix(path, "/v1/members/")
	}
	if input.Disabled != nil && r.Method == "PATCH" {
		if err := s.DB.DisableUser(r.Context(), p, user, *input.Disabled); err != nil {
			cloudFail(w, 403, "disable_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]bool{"saved": true})
		}
		return true
	}
	if input.Reset && r.Method == "PATCH" && s.passwordsEnabled() {
		out, err := s.DB.ResetInvitation(r.Context(), p, user)
		if err != nil {
			cloudFail(w, 403, "reset_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]any{"user": out.User, "url": s.PublicURL + "/ui/#activate=" + out.Token, "expiresAt": out.ExpiresAt})
		}
		return true
	}
	if r.Method == "DELETE" {
		input.Role = ""
	}
	if err := s.DB.ChangeMember(r.Context(), p, user, input.Role); err != nil {
		cloudFail(w, 403, "membership_denied", err.Error())
	} else {
		cloudWrite(w, 200, map[string]bool{"saved": true})
	}
	return true
}

func (s *Server) userAPI(w http.ResponseWriter, r *http.Request, session state.Session) bool {
	if strings.HasPrefix(r.URL.Path, "/v1/user/") {
		if session.Kind != "browser" {
			cloudFail(w, 403, "browser_required", "Use a browser session.")
			return true
		}
		switch {
		case r.URL.Path == "/v1/user/password" && r.Method == "POST" && s.passwordsEnabled():
			var input struct {
				Current  string `json:"current"`
				Password string `json:"password"`
			}
			if decodeCloud(r, &input) != nil {
				cloudFail(w, 400, "invalid_request", "Provide current and new passwords.")
				return true
			}
			if err := s.DB.ChangePassword(r.Context(), session.User, input.Current, input.Password); err != nil {
				cloudFail(w, 400, "password_failed", err.Error())
			} else {
				cloudWrite(w, 200, map[string]bool{"changed": true})
			}
			return true
		case r.URL.Path == "/v1/user/sessions" && r.Method == "GET":
			out, err := s.DB.Sessions(r.Context(), session.User, session.Token)
			if err != nil {
				cloudFail(w, 503, "sessions_unavailable", "Sessions unavailable.")
			} else {
				cloudWrite(w, 200, out)
			}
			return true
		case strings.HasPrefix(r.URL.Path, "/v1/user/sessions/") && r.Method == "DELETE":
			if err := s.DB.RevokeUserSession(r.Context(), session.User, strings.TrimPrefix(r.URL.Path, "/v1/user/sessions/")); err != nil {
				cloudFail(w, 503, "revoke_failed", "Could not revoke session.")
			} else {
				cloudWrite(w, 200, map[string]bool{"revoked": true})
			}
			return true
		}
	}
	if r.URL.Path != "/v1/user" || r.Method != "DELETE" {
		return false
	}
	if session.Kind != "browser" {
		cloudFail(w, 403, "browser_required", "Use a browser session to delete your user.")
		return true
	}
	var input struct {
		ConfirmUser string `json:"confirmUser"`
	}
	if decodeCloud(r, &input) != nil || input.ConfirmUser != session.User {
		cloudFail(w, 400, "confirmation_required", "Confirm your exact user ID.")
		return true
	}
	if err := s.DB.DeleteUser(r.Context(), session.User); err != nil {
		cloudFail(w, 409, "user_delete_denied", err.Error())
	} else {
		cloudWrite(w, 200, map[string]bool{"deleted": true})
	}
	return true
}
