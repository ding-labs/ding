package mcpsetup

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpui"
)

type Session struct {
	Listener                     net.Listener
	URL                          string
	Done                         chan struct{}
	server                       *http.Server
	state, config, nonce, origin string
	deadline                     time.Time
	mu                           sync.Mutex
	complete                     bool
	pair                         func(context.Context, PairOptions, io.Writer) error
}

func NewSession(state, config string) (*Session, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		l.Close()
		return nil, err
	}
	s := &Session{Listener: l, Done: make(chan struct{}), state: state, config: config, nonce: base64.RawURLEncoding.EncodeToString(b), deadline: time.Now().Add(10 * time.Minute), pair: Pair}
	s.origin = "http://" + l.Addr().String()
	s.URL = s.origin + "/#" + s.nonce
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	return s, nil
}

func (s *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	send := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	deny := func() {
		send(http.StatusForbidden, map[string]string{"error": "This setup session is unavailable. Open Ding Setup again."})
	}
	if r.Host != s.Listener.Addr().String() || time.Now().After(s.deadline) {
		deny()
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, mcpui.SetupHTML)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Ding-Setup")), []byte(s.nonce)) != 1 {
		deny()
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/status" {
		_, err := os.Stat(s.config)
		send(200, map[string]any{"state": s.state, "paired": err == nil})
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/pair" || r.Header.Get("Origin") != s.origin {
		deny()
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.complete {
		deny()
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var payload struct {
		State  *string `json:"state"`
		Manage bool    `json:"manage"`
		Retry  bool    `json:"retry"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	bad := func() {
		send(400, map[string]string{"error": "Could not connect. Start the Ding daemon, check its state directory, and use a new connection file if already paired."})
	}
	if media != "application/json" || d.Decode(&payload) != nil {
		bad()
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		bad()
		return
	}
	state := s.state
	if payload.State != nil {
		state = *payload.State
	}
	if strings.HasPrefix(state, "~/") {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, state[2:])
	}
	if !filepath.IsAbs(state) {
		bad()
		return
	}
	if err := s.pair(r.Context(), PairOptions{State: state, Config: s.config, Name: "Ding desktop plugin", Days: 90, Manage: payload.Manage, Retry: payload.Retry}, io.Discard); err != nil {
		bad()
		return
	}
	s.complete = true
	send(200, map[string]bool{"paired": true})
	close(s.Done)
}

func (s *Session) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
	_ = s.Listener.Close()
}

func Run(ctx context.Context, state, config string, out io.Writer) error {
	s, err := NewSession(state, config)
	if err != nil {
		return err
	}
	defer s.Close()
	errors := make(chan error, 1)
	go func() { errors <- s.server.Serve(s.Listener) }()
	fmt.Fprintln(out, "Opening Ding Setup in your browser. This setup window expires in 10 minutes.")
	if err := openBrowser(s.URL); err != nil {
		fmt.Fprintln(out, "Open this private setup link locally: "+s.URL)
	}
	timer := time.NewTimer(time.Until(s.deadline))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.Done:
		return nil
	case <-timer.C:
		return nil
	case err := <-errors:
		return err
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
