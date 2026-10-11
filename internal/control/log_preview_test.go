package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogPreviewUsesLiveClassifierWithoutPersistingText(t *testing.T) {
	mux := http.NewServeMux()
	logPreviewRoute(mux)
	r := httptest.NewRequest("POST", "/v1/tools/log-preview", strings.NewReader(`{"config":{"format":"auto"},"sample":"WARN test\nINFO test\n0 errors\n{\"level\":\"error\"}\n"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var result struct{ Data LogPreview }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || result.Data.Total != 4 || result.Data.Classified != 3 || result.Data.Warnings != 1 || result.Data.Errors != 1 || strings.Contains(w.Body.String(), "0 errors") {
		t.Fatal(w.Code, w.Body.String())
	}
}
