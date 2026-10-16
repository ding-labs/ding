package cloud

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// This protocol exists only in the opt-in test binary. All network exposure is
// localhost or a Docker port published explicitly to the host's loopback.
func scaleExternal(t *testing.T, lab *scaleLab, out string) (func(), func(context.Context) PushBenchmarkResult) {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(key[:])
	start := make(chan struct{})
	ready := make(chan struct{})
	finish := make(chan PushBenchmarkResult)
	lab.controlMu.Lock()
	lab.control = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/lab/ledger":
			if os.Getenv("DING_SCALE_PRIVATE_HOST") == "" || r.Method != "PUT" {
				w.WriteHeader(404)
				return
			}
			path := filepath.Join(out, "requests.jsonl")
			file, err := mcpconfig.CreatePrivate(path)
			if err != nil {
				w.WriteHeader(409)
				return
			}
			_, err = io.Copy(file, http.MaxBytesReader(w, r.Body, 512<<20))
			if err == nil {
				err = file.Sync()
			}
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				os.Remove(path)
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(204)
		case "/lab/start":
			select {
			case start <- struct{}{}:
				<-ready
				w.WriteHeader(204)
			case <-time.After(time.Second):
				w.WriteHeader(409)
			}
		case "/lab/finish":
			var result PushBenchmarkResult
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&result); err != nil {
				w.WriteHeader(400)
				return
			}
			select {
			case finish <- result:
				w.WriteHeader(204)
			case <-time.After(time.Second):
				w.WriteHeader(409)
			}
		default:
			w.WriteHeader(404)
		}
	})
	lab.controlMu.Unlock()
	tokens := map[string]string{}
	for _, p := range lab.producers {
		tokens[p.TokenEnv] = os.Getenv(p.TokenEnv)
	}
	bootstrap := struct {
		URL, Admin, CA string
		Producers      []PushProducer
		Tokens         map[string]string
	}{lab.http.URL, token, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: lab.http.Certificate().Raw})), lab.producers, tokens}
	f, err := mcpconfig.CreatePrivate(filepath.Join(out, "bootstrap.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewEncoder(f).Encode(bootstrap)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	return func() {
			select {
			case <-start:
			case <-time.After(2 * time.Minute):
				t.Fatal("local generator did not start")
			}
		}, func(ctx context.Context) PushBenchmarkResult {
			close(ready)
			select {
			case result := <-finish:
				return result
			case <-ctx.Done():
				t.Fatal("local disk guard stopped the server")
				return PushBenchmarkResult{}
			case <-time.After(time.Hour + 5*time.Minute):
				t.Fatal("local generator did not finish")
				return PushBenchmarkResult{}
			}
		}
}
