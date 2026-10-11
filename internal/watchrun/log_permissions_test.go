package watchrun

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

func TestLogReadersRequireExplicitRevisionGrant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	for _, input := range []string{fmt.Sprintf(`{type: file, path: %q}`, path), fmt.Sprintf(`{type: command, argv: [tail, -f, %q], directory: %q}`, path, dir)} {
		manifest := strings.Replace(nativeLogManifest, "window: 1s", "input: "+input+", window: 1s", 1)
		b, err := plan.Parse([]byte(manifest))
		if err != nil {
			t.Fatal(err)
		}
		g := store.IntegrationGrant{}
		if err := commandAccess(g, b); !errors.Is(err, store.ErrIntegrationDenied) {
			t.Fatal(err)
		}
		g.CommandRevisions = []string{b.Watches[0].Revision}
		if err := commandAccess(g, b); err != nil {
			t.Fatal(err)
		}
	}
}
