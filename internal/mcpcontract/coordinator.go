package mcpcontract

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"slices"
)

// CoordinatorTools have no peer receipt or arbitrary manifest inputs. The
// bridge obtains both from its pinned, authenticated connections.
func CoordinatorTools() []*mcp.Tool {
	key := map[string]any{"type": "string", "minLength": 16, "maxLength": 128, "pattern": "^[A-Za-z0-9_-]+$"}
	connection := map[string]any{"type": "string", "enum": []string{"local", "cloud"}}
	id := map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
	confirmation := map[string]any{"type": "boolean", "const": true, "description": "Set only after the user authorizes this specific reviewed action."}
	out := []*mcp.Tool{}
	for _, entry := range []struct {
		name, description string
		read              bool
		properties        map[string]any
	}{
		{"ding_prepare_transfer", "After inspecting source preflight and user approval of the exact data upload, prepare an eligible watch on the other connection. Source keeps running; target is held paused. No secrets are copied. Reuse this operation key after a lost response.", false, map[string]any{"source_connection": connection, "target_connection": connection, "watch_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "expected_revision": id, "expected_generation": map[string]any{"type": "integer", "minimum": 1}, "expected_destinations": map[string]any{"type": "object", "additionalProperties": id}, "operation_key": key, "upload_confirmed": confirmation}},
		{"ding_test_transfer", "Send or reconcile a labeled test on the saved target. Reuse the operation key; a new key deliberately sends another test. Testing never pauses the source.", false, map[string]any{"transfer_id": id, "operation_key": key}},
		{"ding_finish_transfer", "Only after the user confirms the labeled delivery and authorizes the handoff, fence the source and activate the saved target. Never fabricate a peer proof. After uncertainty inspect this same transfer; never resume the source automatically.", false, map[string]any{"transfer_id": id, "delivery_confirmed": confirmation}},
		{"ding_cancel_transfer", "After user confirmation, cancel only a never-activated target and release any source hold. A paused source remains paused. An active target requires a reviewed reverse transfer.", false, map[string]any{"transfer_id": id, "cancel_confirmed": confirmation}},
		{"ding_get_transfer_status", "Read both saved transfer targets and first-observation confirmation without changing execution. Unknown or unreachable never means stopped.", true, map[string]any{"transfer_id": id}},
	} {
		required := []string{}
		for name := range entry.properties {
			required = append(required, name)
		}
		slices.Sort(required)
		out = append(out, &mcp.Tool{Name: entry.name, Description: entry.description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: entry.read, DestructiveHint: ptr(!entry.read)},
			InputSchema:  map[string]any{"type": "object", "additionalProperties": false, "properties": entry.properties, "required": required},
			OutputSchema: map[string]any{"type": "object", "required": []string{"view", "data"}, "properties": map[string]any{"view": map[string]any{"type": "string", "const": "transfer_summary"}, "data": map[string]any{"type": "object"}}},
		})
	}
	return out
}
