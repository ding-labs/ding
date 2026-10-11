package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func TestCaptureChild(t *testing.T) {
	if os.Getenv("DING_LOG_CAPTURE_HELPER") != "1" {
		return
	}
	for range 1000 {
		fmt.Fprint(os.Stdout, "INFO out\r\n")
		fmt.Fprint(os.Stderr, "WARN err\n")
	}
	io.Copy(os.Stdout, os.Stdin)
	os.Exit(7)
}

func TestExecPreservesOutputInputAndExit(t *testing.T) {
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Minute, MinEvents: 1})
	var out, errout bytes.Buffer
	r := c.Exec(context.Background(), ExecOptions{
		Argv:  []string{os.Args[0], "-test.run=^TestCaptureChild$"},
		Env:   append(os.Environ(), "DING_LOG_CAPTURE_HELPER=1"),
		Stdin: bytes.NewBufferString("final without newline"), Stdout: &out, Stderr: &errout,
	})
	c.Close()
	if r.Err != nil || r.ExitCode != 7 {
		t.Fatal(r)
	}
	if !bytes.Equal(out.Bytes(), append(bytes.Repeat([]byte("INFO out\r\n"), 1000), []byte("final without newline")...)) {
		t.Fatal("stdout changed")
	}
	if !bytes.Equal(errout.Bytes(), bytes.Repeat([]byte("WARN err\n"), 1000)) {
		t.Fatal("stderr changed")
	}
}
