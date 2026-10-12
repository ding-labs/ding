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
	next := roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("a", 70<<10)))}, nil
	})
	transport := budgetTransport{next: next, db: db, workspace: a.ID}
	r, _ := http.NewRequest("GET", "https://example.com", nil)
	response, err := transport.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("unbounded body accepted")
	}
	response.Body.Close()
	response.Body.Close()
	u, err := db.Usage(ctx, a.ID, time.Now())
	if err != nil || u.Checks != 1 || u.Bytes != (64<<10)+1+1024 {
		t.Fatal(u, err)
	}
	r.Header.Set("Authorization", strings.Repeat("x", 9<<10))
	if _, err := transport.RoundTrip(r); err == nil {
		t.Fatal("oversized credential header accepted")
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
	b := (source.HTTP{Client: &http.Client{Transport: budgetTransport{next: next, db: db, workspace: a.ID}}}).Fetch(ctx, p, "", time.Now())
	if len(b.Observations) != 1 || b.Observations[0].Health != "ok" || b.Observations[0].Fields["http.status"] != 503 {
		t.Fatal(b)
	}
	u, err := db.Usage(ctx, a.ID, time.Now())
	if err != nil || u.Checks != 1 || u.Bytes != 1024 {
		t.Fatal("status check did not close and settle its reservation", u, err)
	}
}
