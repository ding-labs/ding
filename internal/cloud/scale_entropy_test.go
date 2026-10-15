package cloud

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"
)

func TestScaleEntropyAndNestedFixtures(t *testing.T) {
	c := scaleConfig{Name: "entropy", Rate: 1, Seconds: 1, Outputs: 18, BodyBytes: 600000, RetainPadding: true, EntropyPadding: true, Nested: true}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	bodies, err := c.bodies()
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies[0]) != c.BodyBytes {
		t.Fatal("incorrect body size")
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, _ = w.Write(bodies[0])
	_ = w.Close()
	if compressed.Len() < len(bodies[0])*7/10 {
		t.Fatal("entropy fixture compresses like repeated padding")
	}
	var body struct{ Events []map[string]any }
	if err = json.Unmarshal(bodies[0], &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 18 || body.Events[0]["workflow_run"] == nil {
		t.Fatal("nested workload missing")
	}
	again, _ := c.bodies()
	if !bytes.Equal(bodies[0], again[0]) {
		t.Fatal("fixture is not reproducible")
	}
}
