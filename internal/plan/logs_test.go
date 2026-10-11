package plan

import (
	"strings"
	"testing"
)

const logManifest = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: output}
spec:
  source: {type: logs, logs: {input: {type: external}}}
  condition: {field: warning_match_ratio, operator: gt, value: 0.5}
`

func TestLogSourceContract(t *testing.T) {
	b, err := Parse([]byte(logManifest))
	if err != nil {
		t.Fatal(err)
	}
	c := b.Watches[0].Definition.Spec.Source.Logs
	if c.Format != "auto" || c.Window != "1m0s" || c.MinEvents != 100 {
		t.Fatal(c)
	}
	for _, replacement := range []string{
		`source: {type: push, logs: {}}`,
		`source: {type: logs, every: 1s, logs: {}}`,
		`source: {type: logs, logs: {format: xml}}`,
		`source: {type: logs, logs: {window: 500ms}}`,
		`source: {type: logs, logs: {minCoverage: 2}}`,
		`source: {type: logs, logs: {input: {type: external, path: /tmp/app.log}}}`,
		`source: {type: logs, logs: {input: {type: file, path: relative.log}}}`,
		`source: {type: logs, logs: {input: {type: file, path: /tmp/app.log, start: beginning}}}`,
		`source: {type: logs, logs: {input: {type: command, argv: [tail], directory: /tmp, reconnectEvery: 1ms}}}`,
	} {
		bad := strings.Replace(logManifest, `source: {type: logs, logs: {input: {type: external}}}`, replacement, 1)
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatal("accepted", replacement)
		}
	}
}
