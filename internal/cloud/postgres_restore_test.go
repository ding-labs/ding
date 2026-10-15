package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestPostgresNativeRestorePreservesReceiptsAndQuarantines(t *testing.T) {
	if os.Getenv("DING_PG_DUMP") == "" || os.Getenv("DING_PG_RESTORE") == "" {
		t.Skip("requires matching native tools")
	}
	ctx := context.Background()
	source := pgtest.New(t)
	source.SetAppEnvironment(t, "RESTORE_SOURCE_APP")
	source.SetAdminEnvironment(t, "RESTORE_SOURCE_ADMIN")
	c := Config{DataDir: t.TempDir(), Database: database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "RESTORE_SOURCE_APP"}, MaxOpenConns: 1}}
	c.KeyFile = filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(c.KeyFile, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(ctx, c, "RESTORE_SOURCE_ADMIN"); err != nil {
		t.Fatal(err)
	}
	s, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := s.Control.Enroll(ctx, "source", "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Control.ConfigureDataKey(ctx, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	tenant, err := s.workspaces.Open(ctx, enrolled.ID)
	if err != nil {
		t.Fatal(err)
	}
	app := watchrun.New(tenant)
	fixture := scaleConfig{Name: "restore", Rate: 1, Seconds: 1}
	if err = fixture.validate(); err != nil {
		t.Fatal(err)
	}
	if _, err = app.Apply(ctx, watchrun.ApplyRequest{Manifest: fixture.manifest()}); err != nil {
		t.Fatal(err)
	}
	record, err := app.Record(ctx, "ci")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := fixture.bodies()
	input := watchrun.NewPushInput("producer", "preserved", body[0], 100, time.Hour)
	batch, err := watchrun.PreparePush(ctx, record, body[0], 32, 32000)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := app.CommitPush(ctx, record, batch, input)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := s.Control.Principal(ctx, enrolled.User, enrolled.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.Control.CreateService(ctx, principal, "restore-test", []string{"inspect"}, []string{"ci"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	tenant.Close()
	s.Close()
	key, _ := age.GenerateX25519Identity()
	backup := filepath.Join(t.TempDir(), "snapshot.age")
	if err = BackupPostgres(ctx, c, "RESTORE_SOURCE_ADMIN", backup, key.Recipient().String()); err != nil {
		t.Fatal(err)
	}
	target := pgtest.New(t)
	target.SetAppEnvironment(t, "RESTORE_TARGET_APP")
	target.SetAdminEnvironment(t, "RESTORE_TARGET_ADMIN")
	c.DataDir = filepath.Join(t.TempDir(), "restored")
	c.Database.DSNRef = &database.SecretRef{Env: "RESTORE_TARGET_APP"}
	if err = RestorePostgres(ctx, c, "RESTORE_TARGET_ADMIN", backup, key.String()); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err = restored.CheckQuarantine(ctx); err == nil {
		t.Fatal("restore allowed to run")
	}
	tenant, err = restored.workspaces.Open(ctx, enrolled.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer tenant.Close()
	if _, err = restored.Control.ServiceToken(ctx, grant.Token); err == nil {
		t.Fatal("restored credential remained valid")
	}
	app = watchrun.New(tenant)
	r, err := app.Record(ctx, "ci")
	if err != nil || r.Status != "paused" {
		t.Fatal("restored watch resumed", err)
	}
	replay, err := app.ReplayPush(ctx, r, input)
	if err != nil || replay.AcceptanceID != receipt.AcceptanceID || replay.First != receipt.First || replay.Last != receipt.Last {
		t.Fatal("receipt changed", err)
	}
	if err = tenant.View(ctx, func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 1 || u.Observations != 1 {
			t.Error("usage changed", u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = tenant.Update(ctx, func(tx *store.Tx) error {
		_, e := tx.AppendObservation("ci", r.Plan.Revision, r.Generation, watch.Observation{InputID: "after-restore", AcceptedAt: time.Now(), Health: "clock"})
		return e
	}); err != nil {
		t.Fatal("restored allocator", err)
	}
}
