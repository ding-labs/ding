package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestLogSessionRoleAndSingleProducer(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	_, err = a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: logs}
spec:
  source: {type: logs, logs: {}}
  condition: {field: warning_match_ratio, operator: gt, value: 0.5}
`})
	if err != nil {
		t.Fatal(err)
	}
	c := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	h := Handler(a, c)
	for _, test := range []struct {
		token  string
		status int
	}{{c.Admin, 401}, {c.Ingest, 200}, {c.Ingest, 409}} {
		r := httptest.NewRequest("POST", "/v1/logs/logs/session", nil)
		r.Header.Set("Authorization", "Bearer "+test.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
