package watchrun

import (
	"errors"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

func TestLogReadersRequireExplicitRevisionGrant(t *testing.T) {
	for _, input := range []string{`{type: file, path: /tmp/app.log}`, `{type: command, argv: [tail, -f, /tmp/app.log], directory: /tmp}`} {
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
