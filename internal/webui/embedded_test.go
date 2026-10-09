//go:build console

package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedReleaseAssets(t *testing.T) {
	h := Handler()
	if h == nil {
		t.Fatal("console build missing handler")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ui/watches/api", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/ui/assets/") {
		t.Fatal("release does not contain console build")
	}
}
