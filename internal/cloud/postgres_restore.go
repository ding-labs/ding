package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloudbackup"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

type postgresRestoreMarker struct {
	Format   string `json:"format"`
	Complete bool   `json:"complete"`
}

func writePostgresRestoreMarker(dir string, complete bool) error {
	path := filepath.Join(dir, cloudbackup.Quarantine)
	if !complete {
		f, err := mcpconfig.CreatePrivate(path)
		if err != nil {
			return err
		}
		err = json.NewEncoder(f).Encode(postgresRestoreMarker{"ding-postgres/v1", false})
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	// Completion is published only after archive authentication and revocation.
	f, err := mcpconfig.CreatePrivate(path + ".complete")
	if err != nil {
		return err
	}
	defer os.Remove(path + ".complete")
	err = json.NewEncoder(f).Encode(postgresRestoreMarker{"ding-postgres/v1", true})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path+".complete", path)
}

// RestorePostgres imports into a fresh directory and an empty migrated database.
// Persistent quarantine survives failed or interrupted imports and directory loss.
func RestorePostgres(ctx context.Context, c Config, adminEnv, input, identity string) error {
	if c.Database.Effective().Backend != "postgres" {
		return fmt.Errorf("PostgreSQL backend required")
	}
	key, err := age.ParseX25519Identity(identity)
	if err != nil {
		return fmt.Errorf("valid age X25519 identity required")
	}
	if !filepath.IsAbs(c.DataDir) {
		return fmt.Errorf("new absolute restore directory required")
	}
	if err = os.Mkdir(c.DataDir, 0700); err != nil {
		return fmt.Errorf("restore directory must not exist")
	}
	// Preserve the directory and incomplete marker after any failure for review.
	if err = writePostgresRestoreMarker(c.DataDir, false); err != nil {
		return err
	}
	if err = MigrateDatabase(ctx, c, adminEnv); err != nil {
		return err
	}
	s, err := openServerStorage(ctx, c.DataDir, c.Database, false)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx = database.WithOwnership(s.Context(), s.owner)
	admin, dsn, err := postgresOperatorPool(ctx, c, adminEnv, s)
	if err != nil {
		return err
	}
	defer admin.Close()
	var count int
	if err = admin.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM ding_control.users)+(SELECT count(*) FROM ding_runtime.workspaces)").Scan(&count); err != nil {
		return database.SafeError("verify empty restore target", err)
	}
	if count != 0 {
		return fmt.Errorf("PostgreSQL restore target must contain no users or workspace state")
	}
	in, err := mcpconfig.OpenPrivate(input)
	if err != nil {
		return err
	}
	defer in.Close()
	decrypted, err := age.Decrypt(in, key)
	if err != nil {
		return fmt.Errorf("backup decryption failed")
	}
	r := bufio.NewReaderSize(decrypted, 4096)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("invalid PostgreSQL backup header")
	}
	var h postgresBackupHeader
	if json.Unmarshal(line, &h) != nil || h.Format != "ding-postgres/v1" || h.Control != state.PostgresSchemaVersion || h.Runtime != store.PostgresSchemaVersion || h.Instance == "" {
		return fmt.Errorf("unsupported PostgreSQL backup schema")
	}
	if err = database.SetQuarantine(ctx, s.pool, s.owner, true); err != nil {
		return err
	}
	if err = postgresTool(ctx, "pg_restore", dsn, r, io.Discard, "--clean", "--if-exists", "--no-owner", "--no-acl", "--single-transaction", "--exit-on-error"); err != nil {
		return err
	}
	// A native archive may stop before the final authenticated age chunk.
	if _, err = io.Copy(io.Discard, r); err != nil {
		return fmt.Errorf("backup authentication failed; restored state remains quarantined")
	}
	role, err := c.Database.Role(nil)
	if err != nil {
		return err
	}
	if err = state.MigratePostgres(ctx, admin, role); err != nil {
		return err
	}
	if err = store.MigratePostgres(ctx, admin, role); err != nil {
		return err
	}
	restored, err := s.Control.InstanceID(ctx)
	if err != nil {
		return err
	}
	if restored != h.Instance {
		return fmt.Errorf("restored instance identity differs from backup header")
	}
	keyBytes, err := loadKey(&c)
	if err != nil {
		return err
	}
	if err = s.Control.ConfigureDataKey(ctx, keyBytes); err != nil {
		return err
	}
	if err = quarantinePostgresState(ctx, s); err != nil {
		return err
	}
	if err = database.BindDirectory(c.DataDir, "postgres", restored); err != nil {
		return err
	}
	return writePostgresRestoreMarker(c.DataDir, true)
}

func quarantinePostgresState(ctx context.Context, s *ServerStorage) error {
	if err := s.Control.QuarantineRestore(ctx); err != nil {
		return err
	}
	workspaces, err := s.Control.Workspaces(ctx)
	if err != nil {
		return err
	}
	deleting, err := s.Control.DeletingWorkspaces(ctx)
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, w := range workspaces {
		ids[w.ID] = true
	}
	for _, id := range deleting {
		ids[id] = true
	}
	for id := range ids {
		tenant, err := s.workspaces.Open(ctx, id)
		if err != nil {
			return err
		}
		err = tenant.QuarantineRestore(ctx)
		closeErr := tenant.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
