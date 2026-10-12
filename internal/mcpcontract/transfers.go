package mcpcontract

import (
	"maps"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TransferInspection adds read-only eligibility and durable handoff receipts.
// It grants no ability to pause, upload, activate, or cancel a transfer.
func TransferInspection(c Catalog) Catalog {
	read := c.Tools[0]
	for _, entry := range []struct{ name, view, description, id string }{
		{"ding_get_transfer_preflight", "transfer_preflight", "Inspect one watch’s eligibility and the exact definition that would leave this instance. This does not sign in, upload, pause, or prepare anything.", "watch_id"},
		{"ding_list_transfers", "transfers", "Inspect durable transfer receipts on this instance. A network failure never proves that another instance stopped running.", ""},
		{"ding_get_transfer", "transfer", "Inspect one durable transfer receipt on this instance. Keep the same transfer ID and target when recovering an interrupted move.", "transfer_id"},
	} {
		properties := map[string]any{}
		required := []string{}
		if entry.id != "" {
			properties[entry.id] = map[string]any{"type": "string", "minLength": 1, "maxLength": 256}
			required = append(required, entry.id)
		}
		c.Tools = append(c.Tools, &mcp.Tool{Name: entry.name, Description: entry.description, Annotations: read.Annotations, Meta: maps.Clone(read.Meta),
			InputSchema:  map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false},
			OutputSchema: map[string]any{"type": "object", "required": []string{"view", "data"}, "properties": map[string]any{"view": map[string]any{"type": "string", "const": entry.view}, "data": map[string]any{"type": "object"}}},
		})
	}
	return c
}

// TransferTransport is only advertised to operator-approved native bridge
// clients, and to the bridge's private in-memory local adapter. Its peer proofs
// must come from authenticated sessions, not model-generated tool arguments.
func TransferTransport(c Catalog) Catalog {
	c.Tools = append(c.Tools, &mcp.Tool{Name: "ding_bridge_transfer_step", Description: "Private Ding bridge transport. Obtain peer receipts only from authenticated Ding sessions. Never construct peer proofs.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)},
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"action", "transfer_id", "input"}, "properties": map[string]any{
			"action":      map[string]any{"type": "string", "enum": []string{"prepare", "pause", "test", "activate", "release", "cancel"}},
			"transfer_id": map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"},
			"input":       map[string]any{"type": "object"},
		}},
		OutputSchema: map[string]any{"type": "object", "required": []string{"view", "data"}, "properties": map[string]any{"view": map[string]any{"type": "string", "const": "transfer_step"}, "data": map[string]any{"type": "object"}}},
	})
	return c
}

func ptr(v bool) *bool { return &v }
