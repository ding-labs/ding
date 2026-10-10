package watchrun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/notify"
)

func TestDesktopDeliveryUsesDurableRetryAndPinnedID(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "type: webhook, urlRef: {env: WEBHOOK_URL}", "type: desktop", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	i, err := a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 3; n++ {
		input(t, a, i.Watch, n, 500)
	}
	now := start.Add(time.Minute)
	a.Now = func() time.Time { return now }
	var ids []string
	a.Notify = func(_ context.Context, m notify.Message) error {
		if m.RequestPermission {
			t.Fatal("background alert requested permission")
		}
		ids = append(ids, m.ID)
		if len(ids) == 1 {
			return errors.New("no desktop session")
		}
		return nil
	}
	if done, err := a.DeliverOne(ctx); err != nil || !done {
		t.Fatal(done, err)
	}
	i, err = a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Deliveries) != 1 || i.Deliveries[0].Status != "pending" {
		t.Fatal(i.Deliveries)
	}
	now = now.Add(time.Minute)
	if done, err := a.DeliverOne(ctx); err != nil || !done {
		t.Fatal(done, err)
	}
	i, err = a.Inspect(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if i.Deliveries[0].Status != "delivered" || len(ids) != 2 || ids[0] != ids[1] {
		t.Fatal(ids, i.Deliveries)
	}
}
