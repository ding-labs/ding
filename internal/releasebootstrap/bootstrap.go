// Package releasebootstrap embeds the hashes into a version-specific installer.
// Initial trust comes from obtaining this script over the official HTTPS channel
// (or inspecting it). Subsequent updates use Ding's pinned Ed25519 trust root.
package releasebootstrap

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/ding-labs/ding/internal/update"
)

//go:embed install.sh.tmpl
var script string

func Render(m update.Manifest) ([]byte, error) {
	t, err := template.New("install").Parse(script)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, m)
	return b.Bytes(), err
}
