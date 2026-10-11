package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestSharedPolicyAllowsPrivateHTTPWithoutWeakeningCloud(t *testing.T) {
	b, err := plan.Parse([]byte(strings.ReplaceAll(fixture, "https://example.com/health", "http://10.0.0.5/health")))
	if err != nil {
		t.Fatal(err)
	}
	if err := sharedPolicy(nil)(b); err != nil {
		t.Fatal(err)
	}
	if err := Policy(b); err == nil {
		t.Fatal("shared profile weakened cloud egress")
	}
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ConfigureHosting(ctx, "self-hosted"); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureHosting(ctx, "cloud"); err == nil {
		t.Fatal("hosting profile changed silently")
	}
}
