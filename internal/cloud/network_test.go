package cloud

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
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
	transport := budgetTransport{next, db, a.ID}
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
