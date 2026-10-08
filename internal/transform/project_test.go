package transform

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProjectionTypesAndBounds(t *testing.T) {
	ctx := context.Background()
	outputs, err := Project(ctx, []byte(`{"events":[{"data":{"ok":true,"n":5,"nothing":null}},{"data":{"ok":false,"n":7}}]}`), ".events[]", map[string]string{"enabled": "data.ok", "value": "data.n", "empty": "data.nothing"}, 2, 1024)
	if err != nil || len(outputs) != 2 || outputs[0]["enabled"] != true || outputs[0]["value"] != float64(5) {
		t.Fatal(outputs, err)
	}
	if _, exists := outputs[1]["empty"]; exists {
		t.Fatal("invented missing field")
	}
	cases := []struct {
		raw, jq      string
		count, bytes int
	}{{`{`, "", 2, 1024}, {`{}`, ".[] | error(\"secret\")", 2, 1024}, {`{}`, "null", 2, 1024}, {`[]`, "", 2, 1024}, {`{"a":{}}`, "", 2, 1024}, {`{}`, "range(0;3)|{n:.}", 2, 1024}, {`{}`, `{value:("x"*100)}`, 2, 32}, {`{}`, "", 0, 1024}, {strings.Repeat("x", 1025), "", 2, 1024}, {string([]byte{'{', '"', 'a', '"', ':', '"', 255, '"', '}'}), "", 2, 1024}, {`{}`, "bad jq @", 2, 1024}}
	for _, tc := range cases {
		if tc.jq == ".[] | error(\"secret\")" {
			tc.jq = `error("secret")`
		}
		if _, err := Project(ctx, []byte(tc.raw), tc.jq, nil, tc.count, tc.bytes); err == nil {
			t.Fatal("accepted", tc)
		}
	}
	if outputs, err := Project(ctx, []byte(`{}`), "empty", nil, 2, 1024); err != nil || len(outputs) != 0 {
		t.Fatal(outputs, err)
	}
	before := time.Now()
	if _, err := Project(ctx, []byte(`{}`), "def forever: forever; forever", nil, 2, 1024); err == nil || time.Since(before) > 2*time.Second {
		t.Fatal("unbounded transform", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Project(canceled, []byte(`{}`), "", nil, 2, 1024); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestProjectionCannotReadEnvironmentOrFiles(t *testing.T) {
	t.Setenv("DING_PRIVATE_TEST", "secret")
	out, err := Project(context.Background(), []byte(`{}`), "env", nil, 10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out[0]) != 0 {
		t.Fatal("jq accessed environment", out)
	}
	if _, err := CompileJQ(`import "/tmp/private" as secret; secret::data`); err == nil {
		t.Fatal("file module import accepted")
	}
}
