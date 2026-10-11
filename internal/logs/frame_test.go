package logs

import (
	"strings"
	"testing"
)

func TestFramingPreservesSplitAndFinalRecords(t *testing.T) {
	var records []string
	f := NewFramer(func(raw []byte, invalid bool) {
		if invalid {
			t.Fatal("unexpected overflow")
		}
		records = append(records, string(raw))
	})
	for _, part := range []string{"WA", "RN x\r", "\nINFO y\n", "final"} {
		f.Write([]byte(part))
	}
	f.Close()
	f.Close()
	if strings.Join(records, "|") != "WARN x\r|INFO y|final" {
		t.Fatal(records)
	}
}

func TestOversizeDrainsOnceAndResumes(t *testing.T) {
	invalid, valid := 0, 0
	f := NewFramer(func(raw []byte, bad bool) {
		if bad {
			invalid++
		} else {
			valid++
			if string(raw) != "ok" {
				t.Fatal(string(raw))
			}
		}
	})
	for range 100 {
		f.Write([]byte(strings.Repeat("x", 4096)))
		if cap(f.buf) > MaxRecordBytes*2 {
			t.Fatal("unbounded partial record")
		}
	}
	f.Write([]byte("\nok\n"))
	f.Write([]byte(strings.Repeat("x", MaxRecordBytes+1)))
	f.Close()
	if invalid != 2 || valid != 1 {
		t.Fatal(invalid, valid)
	}
}
