package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresRestoreQuarantineRevokesAccessAndRefusesSQLiteBackup(t *testing.T) {
	db := postgresTestDB(t)
	ctx := context.Background()
	a, err := db.Enroll(ctx, "issuer", "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := db.CreateService(ctx, owner, "test", []string{"inspect"}, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	session, err := db.NewSession(ctx, a.User, a.ID, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "control.db")
	if err = db.Backup(ctx, path); err == nil {
		t.Fatal("PostgreSQL accepted SQLite backup path")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unsupported backup wrote a file")
	}
	if err = db.QuarantineRestore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ServiceToken(ctx, grant.Token); err == nil {
		t.Fatal("restored credential remained active")
	}
	if _, err = db.Session(ctx, session.Token, time.Now()); err == nil {
		t.Fatal("restored session remained active")
	}
	if _, err = db.Workspace(ctx, a.ID); err != nil {
		t.Fatal("restore quarantine deleted workspace")
	}
}
