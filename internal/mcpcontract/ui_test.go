package mcpcontract

import "testing"

func TestUIResourceCacheKeyTracksBundle(t *testing.T) {
	bind := func(html string) Catalog {
		t.Helper()
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		return BindUI(DestinationSetup(ServicePreview(c)), html)
	}
	first, same, changed := bind("first bundle"), bind("first bundle"), bind("changed bundle")
	uri := first.Resources[1].URI
	if uri == AppURI || uri != same.Resources[1].URI || uri == changed.Resources[1].URI {
		t.Fatal("resource cache key does not follow bundle contents")
	}
	if first.Resources[0].URI != AppURI || first.ResourceContents[0].URI != AppURI {
		t.Fatal("legacy conversation resource removed")
	}
	if first.ResourceContents[1].URI != uri || first.ResourceContents[1].MIMEType != "text/html;profile=mcp-app" {
		t.Fatal("resource descriptor/content mismatch")
	}
	count := 0
	for _, tool := range first.Tools {
		if ui, ok := tool.Meta["ui"].(map[string]any); ok {
			if ui["resourceUri"] != uri {
				t.Fatalf("%s references a stale component", tool.Name)
			}
			count++
		}
	}
	if count == 0 || len(bind("").Resources) != 1 {
		t.Fatal("missing UI tools or headless catalog changed")
	}
}
