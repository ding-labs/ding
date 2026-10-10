// Package webui serves the console compiled into a release binary.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func Serve(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", 405)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/ui/")
		if strings.Contains(name, "..") || strings.Contains(name, "\\") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			if _, err := fs.Stat(files, name); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			first := strings.Split(name, "/")[0]
			switch first {
			case "", "start", "watches", "events", "deliveries", "workbench", "system":
				name = "index.html"
			default:
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
		}
		b, err := fs.ReadFile(files, name)
		if err != nil {
			http.Error(w, "console assets unavailable", 503)
			return
		}
		switch path.Ext(name) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		if r.Method != http.MethodHead {
			_, _ = w.Write(b)
		}
	})
}
