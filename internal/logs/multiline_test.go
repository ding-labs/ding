package logs

import (
	"strings"
	"testing"
	"time"
)

func TestMultilineBoundariesLimitsAndDeadline(t *testing.T) {
	type result struct {
		text    string
		invalid bool
	}
	out := make(chan result, 10)
	f, err := NewConfiguredFramer(FrameConfig{Mode: "multiline", Start: `^(ERROR|WARN)`, Continue: `^\s`, MaxLines: 3, FlushAfter: "100ms"}, func(b []byte, invalid bool) { out <- result{string(b), invalid} })
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write([]byte("ERROR failed\n  at frame\nINFO next\n"))
	if r := <-out; r.invalid || r.text != "ERROR failed\n  at frame" {
		t.Fatal(r)
	}
	if r := <-out; r.text != "INFO next" {
		t.Fatal(r)
	}
	f.Write([]byte("WARN long\n  one\n  two\n  three\n"))
	select {
	case r := <-out:
		if !r.invalid {
			t.Fatal(r)
		}
	case <-time.After(time.Second):
		t.Fatal("multiline deadline did not flush")
	}
	f.Write([]byte("ERROR oversize\n  " + strings.Repeat("x", MaxRecordBytes) + "\nINFO after\n"))
	if r := <-out; !r.invalid {
		t.Fatal(r)
	}
	if r := <-out; r.text != "INFO after" {
		t.Fatal(r)
	}
	f.Write([]byte("WARN final\n  eof"))
	f.Close()
	if r := <-out; r.text != "WARN final\n  eof" {
		t.Fatal(r)
	}
}

func TestCarriageReturnProgressHasExplicitBoundaries(t *testing.T) {
	var lines []string
	f, _ := NewConfiguredFramer(FrameConfig{Mode: "carriage-return"}, func(b []byte, _ bool) {
		if len(b) > 0 {
			lines = append(lines, string(b))
		}
	})
	f.Write([]byte("INFO first\rINFO second\r\nINFO last"))
	f.Close()
	if strings.Join(lines, "|") != "INFO first|INFO second|INFO last" {
		t.Fatal(lines)
	}
}
