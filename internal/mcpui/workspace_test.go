//go:build mcpui

package mcpui

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedWorkspaceMatchesBundle(t *testing.T) {
	data, err := os.ReadFile("dist/workspace.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != WorkspaceHTML {
		t.Fatal("stale embedded workspace")
	}
	if len(data) > 1<<20 || !strings.Contains(WorkspaceHTML, "<title>Ding</title>") || strings.Contains(WorkspaceHTML, "<script src=") {
		t.Fatal("offline bundle contract")
	}
}
