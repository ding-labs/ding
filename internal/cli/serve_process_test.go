//go:build !windows

package cli

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestServeWithOpenStdinStops(t *testing.T) {
	if os.Getenv("DING_SERVE_TEST_HELPER") == "1" {
		if err := runServe(os.Getenv("DING_SERVE_TEST_CONFIG")); err != nil {
			t.Fatal(err)
		}
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	_, port, _ := net.SplitHostPort(addr)
	cfg := filepath.Join(t.TempDir(), "ding.yaml")
	os.WriteFile(cfg, []byte("server:\n  port: "+port+"\n  admin_token: admin-for-process-test\n  ingest_token: ingest-for-process-test\nrules: []\n"), 0600)
	proc := exec.Command(os.Args[0], "-test.run=^TestServeWithOpenStdinStops$")
	proc.Env = append(os.Environ(), "DING_SERVE_TEST_HELPER=1", "DING_SERVE_TEST_CONFIG="+cfg)
	input, err := proc.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if proc.ProcessState == nil {
			proc.Process.Kill()
			proc.Wait()
		}
	}()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	ready := false
	for i := 0; i < 100; i++ {
		r, err := client.Get("http://" + addr + "/health")
		if err == nil {
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon did not listen")
	}
	for _, tc := range []struct {
		path, token string
		want        int
	}{{"/rules", "", 401}, {"/rules", "admin-for-process-test", 200}, {"/ingest", "admin-for-process-test", 401}} {
		req, _ := http.NewRequest("GET", "http://"+addr+tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatal(tc, resp.StatusCode)
		}
	}
	if err := proc.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err, output.String())
		}
	case <-time.After(4 * time.Second):
		proc.Process.Kill()
		<-done
		t.Fatal("SIGTERM blocked with open stdin", output.String())
	}
	if strings.Contains(output.String(), "DATA RACE") {
		t.Fatal(output.String())
	}
}
