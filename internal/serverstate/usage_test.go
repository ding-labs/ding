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
	if _, err := db.sql.Exec("UPDATE usage SET bytes=? WHERE account=?", MonthlyBytes-RequestReservation+1, a.ID); err != nil {
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
