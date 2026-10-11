package state

import (
	"context"
	"testing"
)

func TestLoginAndHostingProfilesRemainPinnedAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureHosting(ctx, "cloud"); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if err := db.ConfigureDataKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ConfigureDataKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	key[0] = 1
	if err := db.ConfigureDataKey(ctx, key); err == nil {
		t.Fatal("changed encryption key")
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"oidc"}); err == nil {
		t.Fatal("changed login policy")
	}
	if err := db.ConfigureHosting(ctx, "self-hosted"); err == nil {
		t.Fatal("changed execution policy")
	}
}
