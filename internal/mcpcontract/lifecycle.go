package mcpcontract

// WatchInspection extends the qualified v1 detail schema. The field is
// optional so a current adapter can still inspect a watch on an older daemon.
func WatchInspection(c Catalog) Catalog {
	for _, tool := range c.Tools {
		if tool.Name != "ding_get_watch" {
			continue
		}
		tool.Description += " Use lifecycle.actions when present. For transfer_held, inspect the same transferId on this target."
		properties := tool.OutputSchema.(map[string]any)["properties"].(map[string]any)
		data := properties["data"].(map[string]any)["properties"].(map[string]any)
		data["lifecycle"] = map[string]any{
			"type": "object", "required": []string{"actions"},
			"properties": map[string]any{
				"actions":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"reason":     map[string]any{"type": "string"},
				"transferId": map[string]any{"type": "string"},
			},
		}
	}
	return c
}
