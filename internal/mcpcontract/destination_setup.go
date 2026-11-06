package mcpcontract

import "github.com/modelcontextprotocol/go-sdk/mcp"

func DestinationSetup(c Catalog) Catalog {
	c.Tools = append(c.Tools, &mcp.Tool{
		Name:        "ding_get_destination_setup",
		Description: "Read notification setup for your current account and selected workspace. Omit destination_ref to get a protected Console setup link, or use an exact saved destination ID. Only confirmed means the user recorded receipt of a labeled test for the current configuration within 24 hours; accepted means provider acceptance only, pending is uncertain, and stale/expired/missing/unavailable/untested/failed require attention. Never ask for webhook secrets in chat or claim receipt yourself. The browser must match the account/workspace shown. After the user returns, refresh this status and obtain a fresh service preview before authorized activation. This tool sends nothing and changes nothing.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"destination_ref": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}},
		OutputSchema: map[string]any{"type": "object", "required": []string{"view", "data"}, "properties": map[string]any{
			"view": map[string]any{"type": "string", "const": "destination_setup"},
			"data": map[string]any{"type": "object", "required": []string{"state", "accountID", "workspaceID", "destinationRef", "setupURL"}, "properties": map[string]any{
				"state":     map[string]any{"type": "string", "enum": []string{"missing", "unavailable", "untested", "pending", "failed", "accepted", "confirmed", "stale", "expired"}},
				"accountID": map[string]any{"type": "string"}, "workspaceID": map[string]any{"type": "string"}, "destinationRef": map[string]any{"type": "string"}, "setupURL": map[string]any{"type": "string"},
			}},
		}},
	})
	return c
}
