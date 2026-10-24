package state

import (
	"context"
	"testing"
)

func checkInitialDisplayName(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	e, err := db.Enroll(ctx, "issuer", "person", 10)
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.User(ctx, e.User)
	if err != nil || u.Name != "" {
		t.Fatal("enrollment invented a display name", u, err)
	}
	if err := db.InitializeDisplayName(ctx, e.User, "  Zoë Studios  "); err != nil {
		t.Fatal(err)
	}
	if err := db.InitializeDisplayName(ctx, e.User, "Another linked sign-in"); err != nil {
		t.Fatal(err)
	}
	u, err = db.User(ctx, e.User)
	if err != nil || u.Name != "Zoë Studios" {
		t.Fatal("later sign-in replaced the chosen name", u, err)
	}
}

func TestInitialDisplayName(t *testing.T) {
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checkInitialDisplayName(t, db)
}

func TestPostgresInitialDisplayName(t *testing.T) {
	checkInitialDisplayName(t, postgresTestDB(t))
}
