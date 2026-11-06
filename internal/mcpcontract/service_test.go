package mcpcontract

import (
	"maps"
	"testing"
)

func TestServicePreviewSchema(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		catalog, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		catalog = ServicePreview(catalog)
		if cloud {
			catalog = Cloud(catalog)
		}
		found := false
		for _, tool := range catalog.Tools {
			if tool.Name != "ding_preview_service_watch" {
				continue
			}
			found = true
			schema, err := Resolve(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"watch_id": "service", "url": "https://example.com", "destination_ref": "alerts"}
			if err := schema.Validate(args); err != nil {
				t.Fatal(err)
			}
			for _, invalid := range []map[string]any{{"expected_status": float64(99)}, {"expected_status": float64(600)}, {"expected_status": float64(204.5)}, {"expected_status": "204"}, {"expected_status": nil}, {"watch_id": ""}, {"every": "1s"}} {
				bad := maps.Clone(args)
				maps.Copy(bad, invalid)
				if schema.Validate(bad) == nil {
					t.Fatal("schema accepted invalid starter", invalid)
				}
			}
			args["expected_status"] = float64(204)
			if err := schema.Validate(args); err != nil {
				t.Fatal(err)
			}
			args["workspace_id"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			if err := schema.Validate(args); (err == nil) != cloud {
				t.Fatal("workspace boundary", cloud, err)
			}
		}
		if !found {
			t.Fatal("typed service tool missing")
		}
	}
}
