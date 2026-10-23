package cloud

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/watch"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCloudNetworkAccountsBytesAndRejectsOversizedBodies(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Enroll(ctx, "issuer", "subject", 1)
	if err != nil {
		t.Fatal(err)
	}
	var charged int64
	for i, tc := range []struct {
		name string
		size int
	}{
		{"small API", 1536},
		{"above old limit", 70 << 10},
		{"at limit", 1 << 20},
		{"over limit", (1 << 20) + 1},
		{"bounded overflow", 2 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("a", tc.size)))}, nil
			})
			transport := budgetTransport{next: next, db: db, workspace: a.ID}
			r, _ := http.NewRequest("GET", "https://example.com", nil)
			response, err := transport.RoundTrip(r)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			if (err != nil) != (tc.size > 1<<20) {
				t.Fatalf("read %d bytes: %v", len(body), err)
			}
			wantRead := min(tc.size, (1<<20)+1)
			if len(body) != wantRead {
				t.Fatalf("read %d bytes, want %d", len(body), wantRead)
			}
			response.Body.Close()
			response.Body.Close()
			charged += int64(wantRead + 1024)
			u, err := db.Usage(ctx, a.ID, time.Now())
			if err != nil || u.Checks != i+1 || u.Bytes != charged {
				t.Fatal("incorrect network charge", u, err)
			}
			r.Header.Set("Authorization", strings.Repeat("x", 9<<10))
			if _, err := transport.RoundTrip(r); err == nil {
				t.Fatal("oversized credential header accepted")
			}
		})
	}
}

func TestCloudStatusOnlyCheckSettlesBudgetWithoutReadingLargeBody(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Enroll(ctx, "issuer", "subject", 1)
	if err != nil {
		t.Fatal(err)
	}
	next := roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("a", 140<<10)))}, nil
	})
	p, err := plan.Compile(watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "status"}, Spec: watch.Spec{
		Source:    watch.Source{Type: "http", URL: "https://example.com"},
		Condition: watch.Condition{Field: "http.status", Operator: "gte", Value: 400},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Policy(plan.Bundle{Watches: []plan.Compiled{p}}); err != nil {
		t.Fatal("default status watch rejected before acquisition", err)
	}
	b := (source.HTTP{Client: &http.Client{Transport: budgetTransport{next: next, db: db, workspace: a.ID}}}).Fetch(ctx, p, "", time.Now())
	if len(b.Observations) != 1 || b.Observations[0].Health != "ok" || b.Observations[0].Fields["http.status"] != 503 {
		t.Fatal(b)
	}
	u, err := db.Usage(ctx, a.ID, time.Now())
	if err != nil || u.Checks != 1 || u.Bytes != 1024 {
		t.Fatal("status check did not close and settle its reservation", u, err)
	}
}

func TestCloudHTTPDefaultLimitsAllowJSONProjection(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Enroll(ctx, "issuer", "subject", 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Compile(watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "extraction-health"}, Spec: watch.Spec{
		Source:    watch.Source{Type: "http", URL: "https://freedesign.md/api/extraction-health", Fields: map[string]string{"total": "total"}},
		Condition: watch.Condition{Field: "total", Operator: "gte", Value: 200},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Policy(plan.Bundle{Watches: []plan.Compiled{p}}); err != nil {
		t.Fatal("default JSON watch rejected before acquisition", err)
	}
	var charged int64
	for i, size := range []int{1536, 70 << 10, 1 << 20} {
		const prefix, suffix = `{"total":203,"padding":"`, `"}`
		body := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
		next := roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		b := (source.HTTP{Client: &http.Client{Transport: budgetTransport{next: next, db: db, workspace: a.ID}}}).Fetch(ctx, p, "", time.Now())
		if len(b.Observations) != 1 || b.Observations[0].Health != "ok" || b.Observations[0].Fields["total"] != float64(203) {
			t.Fatalf("%d-byte JSON projection failed: %+v", size, b)
		}
		charged += int64(size + 1024)
		u, err := db.Usage(ctx, a.ID, time.Now())
		if err != nil || u.Checks != i+1 || u.Bytes != charged {
			t.Fatal("projection did not settle its network charge", u, err)
		}
	}
}
