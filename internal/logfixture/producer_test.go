package logfixture

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestProducerHasFixedBytesAndIndependentCounts(t *testing.T) {
	for _, format := range []string{"text", "jsonl"} {
		var out, errout bytes.Buffer
		r, err := Produce(&out, &errout, 1000, 1024, 10*time.Millisecond, format)
		if err != nil || r.Records != 10 || r.Warnings != 10 || r.Bytes != 10240 || out.Len() != 5120 || errout.Len() != 5120 {
			t.Fatal(r, err)
		}
		if format == "jsonl" {
			for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
				if !json.Valid(line) {
					t.Fatal("invalid fixture JSON")
				}
			}
		}
	}
}
