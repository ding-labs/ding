package state

import (
	"context"
	"testing"
)

func TestExplicitOIDCLinkPreservesAccountWithClosedEnrollment(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	owner, err := db.Bootstrap(ctx, "owner", "Owner", "a correct long password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"password", "oidc"}); err == nil {
		t.Fatal("ordinary startup enabled OIDC")
	}
	if _, err := db.Enroll(ctx, "https://auth.example", "subject", 0); err == nil {
		t.Fatal("closed enrollment admitted new account")
	}
	if err := db.EnableOIDC(ctx, IdentityLink{User: "missing", Issuer: "https://auth.example", Subject: "subject"}); err == nil {
		t.Fatal("linked missing user")
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal("failed link changed login methods", err)
	}
	link := IdentityLink{User: owner.User, Issuer: "https://auth.example", Subject: "subject"}
	for range 2 {
		if err := db.EnableOIDC(ctx, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ConfigureLoginMethods(ctx, []string{"password", "oidc"}); err != nil {
		t.Fatal(err)
	}
	enrolled, err := db.Enroll(ctx, link.Issuer, link.Subject, 0)
	if err != nil || enrolled.ID != owner.ID || enrolled.User != owner.User {
		t.Fatal("OIDC created a different workspace", enrolled, err)
	}
	if err := db.ConfigureOIDCIssuer(ctx, "https://other.example"); err == nil {
		t.Fatal("issuer replacement accepted")
	}
	if err := db.EnableOIDC(ctx, IdentityLink{User: owner.User, Issuer: "https://other.example", Subject: "subject"}); err == nil {
		t.Fatal("link replaced pinned issuer")
	}
	if _, err := db.Enroll(ctx, link.Issuer, "unlinked", 0); err == nil {
		t.Fatal("identity linking opened public signup")
	}
	var count int
	_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&count)
	if count != 1 {
		t.Fatal("workspace duplicated", count)
	}
}
