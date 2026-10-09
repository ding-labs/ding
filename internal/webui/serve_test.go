package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticRouting(t *testing.T) {
	h := Serve(fstest.MapFS{"index.html": {Data: []byte("<main>ding</main>")}, "assets/app-123.js": {Data: []byte("console.log('ding')")}})
	for _, tc := range []struct {
		path string
		code int
		kind string
	}{{"/ui/", 200, "text/html"}, {"/ui/watches/api", 200, "text/html"}, {"/ui/assets/app-123.js", 200, "text/javascript"}, {"/ui/assets/missing.js", 404, ""}, {"/ui/unknown", 404, ""}, {"/ui/../tokens.json", 404, ""}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.code || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.kind) {
			t.Fatal(tc, w.Code, w.Header())
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("no CSP")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/ui/", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
