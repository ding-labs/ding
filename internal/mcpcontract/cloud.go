package mcpcontract

import (
	"maps"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Target struct {
	Instance  string `json:"instance"`
	Workspace string `json:"workspace"`
	Execution string `json:"execution"`
}

type Workspace struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	Execution       string   `json:"execution"`
	EffectiveScopes []string `json:"effectiveScopes"`
	GrantExpiresAt  string   `json:"grantExpiresAt"`
}

// Cloud extends the v1 watch contract additively. Local tools keep their original
// schemas and reject workspace arguments instead of silently ignoring them.
func Cloud(c Catalog) Catalog {
	read := c.Tools[0]
	for _, entry := range []struct {
		name, description, view string
		data                    map[string]any
	}{
		{"ding_list_workspaces", "List only your approved, usable workspaces for this model client. Choose workspace_id explicitly when more than one is available.", "workspaces", map[string]any{"type": "object", "required": []string{"workspaces"}, "properties": map[string]any{"workspaces": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"id", "name", "role", "execution", "effectiveScopes", "grantExpiresAt"}}}}}},
		{"ding_get_cloud_status", "Inspect cloud usage, monthly limits, reset time and execution health for the selected workspace.", "cloud_status", map[string]any{"type": "object", "required": []string{"usage", "limits", "resetsAt", "health", "policy"}}},
	} {
		c.Tools = append(c.Tools, &mcp.Tool{Name: entry.name, Description: entry.description, Annotations: read.Annotations, Meta: maps.Clone(read.Meta),
			InputSchema:  map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
			OutputSchema: map[string]any{"type": "object", "required": []string{"view", "data"}, "properties": map[string]any{"view": map[string]any{"const": entry.view, "type": "string"}, "data": entry.data}},
		})
	}
	for _, tool := range c.Tools {
		if tool.Name != "ding_list_workspaces" {
			properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
			properties["workspace_id"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$", "description": "Exact approved workspace ID. Required when this client has more than one usable workspace."}
		}
	}
	return c
}
