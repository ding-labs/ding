package mcpcontract

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Profile identifies the authenticated account, independently of workspace,
// session, consent, or editable labels. It deliberately has no View envelope.
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func ProfileTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "ding_get_profile",
		Description: "Identify the connected Ding account before choosing a workspace. Its opaque ID stays the same across reconnects and name changes. This does not change accounts or permissions.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		OutputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"id":   map[string]any{"type": "string", "minLength": 1, "pattern": `\S`},
			"name": map[string]any{"type": "string"},
		}, "required": []string{"id"}, "additionalProperties": false},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), OpenWorldHint: new(false)},
		Meta:        mcp.Meta{"openai/profile": true},
	}
}
