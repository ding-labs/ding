package cloud

import (
	"net/http"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func (s *Server) workspaceSettings(w http.ResponseWriter, r *http.Request, principal access.Principal) {
	if principal.Kind != "browser" {
		cloudFail(w, 403, "browser_required", "Use a browser session to rename your workspace.")
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if decodeCloud(r, &input) != nil {
		cloudFail(w, 400, "invalid_workspace", "Provide a workspace name.")
		return
	}
	name, err := identity.DisplayName(input.Name)
	if err != nil {
		cloudFail(w, 400, "invalid_workspace", "Workspace name must contain 1–100 characters on one line.")
		return
	}
	workspace, err := s.DB.RenameWorkspace(r.Context(), principal, name)
	if err != nil {
		cloudFail(w, 503, "workspace_unavailable", "Could not save the workspace name. Try again.")
		return
	}
	cloudWrite(w, 200, workspace)
}
