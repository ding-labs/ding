package mcpcontract

import (
	"crypto/sha256"
	"fmt"
)

// BindUI gives each bundled component a stable, content-addressed cache key.
// Hosts cache resource HTML by URI, independently of the server version.
// Retain the original URI for conversations created before versioned resources.
// The catalog must be freshly loaded: like the other catalog adapters, this
// function updates its descriptors in place.
func BindUI(c Catalog, html string) Catalog {
	if html == "" {
		return c
	}
	uri := fmt.Sprintf("ui://ding/workspace-%x.html", sha256.Sum256([]byte(html)))
	for _, tool := range c.Tools {
		if ui, ok := tool.Meta["ui"].(map[string]any); ok && ui["resourceUri"] == AppURI {
			ui["resourceUri"] = uri
		}
	}
	resource := *c.Resources[0]
	content := *c.ResourceContents[0]
	resource.URI, content.URI = uri, uri
	c.Resources = append(c.Resources, &resource)
	c.ResourceContents = append(c.ResourceContents, &content)
	return c
}
