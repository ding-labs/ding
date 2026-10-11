package logs

import "testing"

func TestJSONAliasesAreExplicitAndDoNotChangeUnknownNumbers(t *testing.T) {
	p, err := NewParser(ParserConfig{Format: "jsonl", LevelMap: map[string]string{"40": "warning", "problem": "error"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		line  string
		level Level
	}{{`{"level":40}`, Warning}, {`{"level":"problem"}`, Error}, {`{"level":50}`, Unclassified}, {`{"level":"INFO"}`, Info}} {
		if c := p.Classify([]byte(tc.line)); c.Invalid || c.Level != tc.level {
			t.Fatal(tc, c)
		}
	}
	if _, err := NewParser(ParserConfig{LevelMap: map[string]string{"40": "not-a-level"}}); err == nil {
		t.Fatal("invalid alias accepted")
	}
}
