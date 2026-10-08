package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type credentials struct {
	Admin  string `json:"admin"`
	Ingest string `json:"ingest"`
}

// SecureHandler prepares daemon authentication. The token file is private and
// only created for local defaults; remote bindings require explicit tokens.
// Handler() is the trusted in-process handler used by embedding code and tests.
func (s *Server) SecureHandler() (http.Handler, error) {
	s.mu.RLock()
	cfg := s.cfg.Server
	s.mu.RUnlock()
	creds := credentials{Admin: cfg.AdminToken, Ingest: cfg.IngestToken}
	if creds.Admin == "" || creds.Ingest == "" {
		path := cfg.TokenFile
		if path == "" {
			dir, err := os.UserConfigDir()
			if err != nil {
				return nil, err
			}
			path = filepath.Join(dir, "ding", "legacy-tokens.json")
		}
		saved, err := loadCredentials(path)
		if err != nil {
			return nil, err
		}
		if creds.Admin == "" {
			creds.Admin = saved.Admin
		}
		if creds.Ingest == "" {
			creds.Ingest = saved.Ingest
		}
	}
	if len(creds.Admin) < 16 || len(creds.Ingest) < 16 || creds.Admin == creds.Ingest {
		return nil, fmt.Errorf("separate admin and ingest tokens of at least 16 characters are required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			want := creds.Admin
			if r.URL.Path == "/ingest" {
				want = creds.Ingest
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if token == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		s.mux.ServeHTTP(w, r)
	}), nil
}
func loadCredentials(path string) (credentials, error) {
	var c credentials
	data, err := os.ReadFile(path)
	if err == nil {
		info, e := os.Stat(path)
		if e != nil {
			return c, e
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return c, fmt.Errorf("token file must be private (mode 0600)")
		}
		if json.Unmarshal(data, &c) != nil || len(c.Admin) < 16 || len(c.Ingest) < 16 || c.Admin == c.Ingest {
			return c, fmt.Errorf("invalid token file; preserve it and repair explicitly")
		}
		return c, nil
	}
	if !os.IsNotExist(err) {
		return c, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return c, err
	}
	newToken := func() string { var b [32]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
	c = credentials{Admin: newToken(), Ingest: newToken()}
	data, _ = json.Marshal(c)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return loadCredentials(path)
	}
	if err != nil {
		return c, err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return c, err
}
