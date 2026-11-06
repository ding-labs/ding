package mcpcontract

import "maps"

// ServicePreview adds a typed entry point without changing the captured generic
// manifest contract. Cloud adds workspace selection after this decoration.
func ServicePreview(c Catalog) Catalog {
	for _, tool := range c.Tools {
		if tool.Name != "ding_preview_changes" {
			continue
		}
		starter := *tool
		starter.Name = "ding_preview_service_watch"
		starter.Description = "Preview a new HTTP service watch using an existing destination. Checks every 5 minutes in Cloud or 30 seconds locally; fires after 3 failures and recovers after 2 healthy checks. By default HTTP 5xx is failure; expected_status instead requires that exact status. Network errors hold the incident and produce source-error events. This does not fetch the URL, test delivery, or activate a watch. Review the returned manifest and preview, then use ding_apply_changes only when authorized. Existing watch IDs are rejected."
		starter.Meta = maps.Clone(tool.Meta)
		starter.InputSchema = map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"watch_id", "url", "destination_ref"},
			"properties": map[string]any{
				"watch_id":        map[string]any{"type": "string", "minLength": 1, "description": "ID for a new watch."},
				"url":             map[string]any{"type": "string", "minLength": 1, "description": "HTTP or HTTPS service URL. Cloud requires a public destination."},
				"destination_ref": map[string]any{"type": "string", "minLength": 1, "description": "Exact existing destination ID from ding_list_destinations. Never a credential or webhook URL."},
				"expected_status": map[string]any{"type": "integer", "minimum": 100, "maximum": 599, "description": "Optional exact healthy HTTP status, for example 204. Omit to alert on HTTP 5xx."},
			},
		}
		c.Tools = append(c.Tools, &starter)
		break
	}
	return c
}
