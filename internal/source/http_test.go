package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

func compiled(t *testing.T, url string, edit func(*watch.Definition)) plan.Compiled {
	t.Helper()
	d := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "test"}, Spec: watch.Spec{Source: watch.Source{Type: "http", URL: url}, Condition: watch.Condition{Field: "http.status", Operator: "gte", Value: 500}}}
	if edit != nil {
		edit(&d)
	}
	p, err := plan.Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestHTTPStatusProjectionAndConditional(t *testing.T) {
	n := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing header")
		}
		if n == 2 {
			if r.Header.Get("If-None-Match") != "v1" {
				t.Error("lost checkpoint")
			}
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", "v1")
		w.WriteHeader(503)
		fmt.Fprint(w, `{"data":{"healthy":false,"count":3,"empty":null},"secret":"ignored"}`)
	}))
	defer s.Close()
	p := compiled(t, s.URL, func(d *watch.Definition) {
		d.Spec.Source.Fields = map[string]string{"healthy": "data.healthy", "count": "data.count", "empty": "data.empty", "missing": "absent"}
		d.Spec.Source.Headers = map[string]watch.SecretRef{"Authorization": {Env: "AUTH"}}
	})
	h := HTTP{Client: s.Client(), Lookup: func(string) (string, bool) { return "Bearer secret", true }}
	b := h.Fetch(context.Background(), p, "", time.Now())
	if len(b.Observations) != 1 || b.Observations[0].Health != "ok" {
		t.Fatal(b)
	}
	fields := b.Observations[0].Fields
	if fields["http.status"] != 503 || fields["healthy"] != false || fields["count"] != float64(3) {
		t.Fatal(fields)
	}
	if _, ok := fields["missing"]; ok {
		t.Fatal("invented missing field")
	}
	if _, ok := fields["secret"]; ok {
		t.Fatal("retained body")
	}
	b = h.Fetch(context.Background(), p, b.Cursor, time.Now())
	if b.Observations[0].Health != "unchanged" || b.Observations[0].Fields != nil {
		t.Fatal(b)
	}
}
func TestHTTPFailureContracts(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, body, reason string
		status             int
		fields             map[string]string
		max                int
	}{
		{name: "real500", status: 500},
		{name: "redirect", status: 302, reason: "source_redirect_rejected"},
		{name: "badjson", status: 200, body: "bad", fields: map[string]string{"a": "a"}, reason: "source_invalid_projection"},
		{name: "nested", status: 200, body: `{"a":{}}`, fields: map[string]string{"a": "a"}, reason: "source_invalid_projection"},
		{name: "bound", status: 200, body: strings.Repeat("x", 2000), max: 1024, reason: "source_response_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			p := compiled(t, s.URL, func(d *watch.Definition) {
				d.Spec.Source.Fields = tc.fields
				if tc.max > 0 {
					d.Spec.Limits.MaxBytes = tc.max
				}
			})
			b := (HTTP{Client: s.Client()}).Fetch(context.Background(), p, "", now)
			if b.Observations[0].Detail != tc.reason || !b.RetryAt.Equal(now.Add(time.Minute)) {
				t.Fatal(b)
			}
			if tc.reason != "" && b.Observations[0].Health != "unknown" {
				t.Fatal(b)
			}
		})
	}
	p := compiled(t, "http://127.0.0.1:1", nil)
	h := HTTP{}
	for _, tc := range []struct {
		edit           func()
		cursor, reason string
	}{
		{cursor: "bad", reason: "invalid_checkpoint"},
		{reason: "source_transport_failed"},
		{edit: func() { p.Definition.Spec.Source.URLRef = &watch.SecretRef{Env: "SECRET"} }, reason: "missing_credentials"},
		{edit: func() { h.Lookup = func(string) (string, bool) { return "file:///private", true } }, reason: "invalid_endpoint"},
	} {
		if tc.edit != nil {
			tc.edit()
		}
		b := h.Fetch(context.Background(), p, tc.cursor, now)
		if b.Observations[0].Detail != tc.reason {
			t.Fatal(b)
		}
	}
}
func TestHTTPTimeoutIsUnknown(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	p := compiled(t, s.URL, func(d *watch.Definition) { d.Spec.Source.Timeout = "10ms" })
	b := (HTTP{Client: s.Client()}).Fetch(context.Background(), p, "", time.Now())
	if b.Observations[0].Health != "unknown" || b.Observations[0].Fields != nil {
		t.Fatal("timeout became status", b)
	}
}

func TestHTTPProjectionMultipleOutputsAndReservedStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		fmt.Fprint(w, `{"events":[{"n":1},{"n":2}]}`)
	}))
	defer s.Close()
	p := compiled(t, s.URL, func(d *watch.Definition) { d.Spec.Source.JQ = `.events[] | {value:.n,"http.status":200}` })
	b := (HTTP{Client: s.Client()}).Fetch(context.Background(), p, "", time.Now())
	if len(b.Observations) != 2 || b.Observations[1].Fields["value"] != float64(2) || b.Observations[0].Fields["http.status"] != 503 {
		t.Fatal(b)
	}
	p.Definition.Spec.Source.JQ = "empty"
	b = (HTTP{Client: s.Client()}).Fetch(context.Background(), p, "", time.Now())
	if len(b.Observations) != 1 || b.Observations[0].Health != "unchanged" {
		t.Fatal(b)
	}
}

func TestSourceObservedTimeIsExplicitMetadata(t *testing.T) {
	observations, err := Observations([]map[string]any{{"value": 7, "timestamp": "2001-01-01T00:00:00Z"}}, "timestamp")
	if err != nil || observations[0].ObservedAt == nil || observations[0].ObservedAt.Year() != 2001 {
		t.Fatal(observations, err)
	}
	for _, fields := range []map[string]any{{}, {"timestamp": 123}, {"timestamp": "bad"}} {
		if _, err := Observations([]map[string]any{fields}, "timestamp"); err == nil {
			t.Fatal("bad timestamp accepted")
		}
	}
}

func TestInvalidTimestampDoesNotAdvanceCache(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "invalid-time")
		fmt.Fprint(w, `{"timestamp":"bad"}`)
	}))
	defer s.Close()
	p := compiled(t, s.URL, func(d *watch.Definition) {
		d.Spec.Source.Fields = map[string]string{"time": "timestamp"}
		d.Spec.Source.ObservedAtField = "time"
	})
	old := `{"etag":"known-valid"}`
	b := (HTTP{Client: s.Client()}).Fetch(context.Background(), p, old, time.Now())
	if b.Observations[0].Detail != "source_invalid_observed_time" || b.Cursor != old {
		t.Fatal("cached invalid representation", b)
	}
}
func TestUnsolicited304IsUnknown(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(304) }))
	defer s.Close()
	p := compiled(t, s.URL, nil)
	b := (HTTP{Client: s.Client()}).Fetch(context.Background(), p, "", time.Now())
	if b.Observations[0].Detail != "source_unexpected_304" {
		t.Fatal(b)
	}
}
