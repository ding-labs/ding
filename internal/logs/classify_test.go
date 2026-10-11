package logs

import "testing"

func TestClassification(t *testing.T) {
	p, err := NewParser(ParserConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		line string
		want Classification
	}{
		{"[WARN] slow query", Classification{Level: Warning}},
		{"2026-10-11T12:00:00Z ERROR failed", Classification{Level: Error}},
		{"2026/10/11 12:00:00 INFO started", Classification{Level: Info}},
		{"12:00:00.123 WARNING: retrying", Classification{Level: Warning}},
		{"\x1b[31mERROR broken\x1b[0m\r", Classification{Level: Error}},
		{`time=now level=warn msg="slow"`, Classification{Level: Warning}},
		{`{"level":"fatal","message":"bye"}`, Classification{Level: Fatal}},
		{`{"message":"ordinary output"}`, Classification{}},
		{"{non-json output}", Classification{}},
		{"0 errors found", Classification{}},
		{"ERRORS: zero", Classification{}},
		{`msg="example level=error text"`, Classification{}},
		{"normal stderr output", Classification{}},
		{"\xff", Classification{Invalid: true}},
		{" \r\n", Classification{Blank: true}},
	} {
		if got := p.Classify([]byte(test.line)); got != test.want {
			t.Errorf("%q: got %+v, want %+v", test.line, got, test.want)
		}
	}
}

func TestExplicitJSONAndPatterns(t *testing.T) {
	p, _ := NewParser(ParserConfig{Format: "jsonl", LevelField: "log.severity"})
	if got := p.Classify([]byte(`{"log":{"severity":"warn"}}`)); got.Level != Warning {
		t.Fatal(got)
	}
	if got := p.Classify([]byte("not JSON")); !got.Invalid {
		t.Fatal(got)
	}
	if got := p.Classify([]byte(`[]`)); !got.Invalid {
		t.Fatal(got)
	}
	p, _ = NewParser(ParserConfig{Patterns: map[string]string{"error": "^failed:", "warning": "^failed: retry"}})
	if got := p.Classify([]byte("failed: retry")); got.Level != Error {
		t.Fatal("overlapping rules must use highest severity", got)
	}
	for _, c := range []ParserConfig{{Format: "xml"}, {Patterns: map[string]string{"other": "x"}}, {Patterns: map[string]string{"WARN": "x"}}, {Patterns: map[string]string{"error": "["}}} {
		if _, err := NewParser(c); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}
