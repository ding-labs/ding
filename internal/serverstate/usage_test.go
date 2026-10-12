package state

import (
	"context"
	"testing"
	"time"
)

func TestBudgetReservationCannotOverbookOrRefundTwice(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	now := time.Now()
	r, err := db.Reserve(ctx, a.ID, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Finish(ctx, r, 100); err != nil {
		t.Fatal(err)
	}
	if err := db.Finish(ctx, r, 100); err != nil {
		t.Fatal(err)
	}
	u, err := db.Usage(ctx, a.ID, now)
	if err != nil || u.Checks != 1 || u.Bytes != 100 {
		t.Fatal(u, err)
	}
	if _, err := db.sql.Exec("UPDATE usage SET bytes=? WHERE workspace=?", MonthlyBytes-RequestReservation+1, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Reserve(ctx, a.ID, false, now); err == nil {
		t.Fatal("overbooked byte budget")
	}
	if _, err := db.Reserve(ctx, b.ID, false, now); err != nil {
		t.Fatal("one workspace exhausted another", err)
	}
	if _, err := db.Reserve(ctx, a.ID, false, now.AddDate(0, 1, 0)); err != nil {
		t.Fatal("new month did not reset", err)
	}
}

func TestConfiguredOutboundBudgetsAndUnmeteredQuotaMode(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace, err := db.Enroll(ctx, "issuer", "customer", 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	bounded := OutboundBudget{Checks: 1, Deliveries: 1, Bytes: 2 * RequestReservation}
	r, err := db.ReserveBudget(ctx, workspace.ID, false, now, bounded)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Finish(ctx, r, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveBudget(ctx, workspace.ID, false, now, bounded); err == nil {
		t.Fatal("configured check quota ignored")
	}
	for i := 0; i < 4; i++ {
		if _, err := db.ReserveBudget(ctx, workspace.ID, false, now, OutboundBudget{}); err != nil {
			t.Fatal("zero quota blocked metered traffic", err)
		}
	}
	if _, err := db.ReserveBudget(ctx, workspace.ID, false, now, OutboundBudget{}); err == nil {
		t.Fatal("disabling cumulative quota bypassed concurrency")
	}
	usage, err := db.Usage(ctx, workspace.ID, now)
	if err != nil || usage.Checks != 5 {
		t.Fatal("optional quota disabled accounting", usage, err)
	}
}
