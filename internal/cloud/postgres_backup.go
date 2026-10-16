package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/jackc/pgx/v5"
)

type postgresBackupHeader struct {
	Format    string    `json:"format"`
	Control   int       `json:"controlSchema"`
	Runtime   int       `json:"runtimeSchema"`
	Instance  string    `json:"instance"`
	CreatedAt time.Time `json:"createdAt"`
}

// postgresOperatorPool proves that the separately supplied administrative DSN
// points at this owned instance. Neither DSN is passed in process arguments.
func postgresOperatorPool(ctx context.Context, c Config, env string, s *ServerStorage) (*sql.DB, string, error) {
	if env == "" {
		return nil, "", fmt.Errorf("PostgreSQL backup and restore require --database-admin-env")
	}
	admin := c.Database
	admin.DSNRef = &database.SecretRef{Env: env}
	admin.MaxOpenConns = 1
	dsn, err := admin.ResolveDSN(nil)
	if err != nil {
		return nil, "", err
	}
	pool, err := database.OpenPostgres(ctx, admin, nil)
	if err != nil {
		return nil, "", err
	}
	var bypass bool
	if err = pool.QueryRowContext(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&bypass); err != nil || !bypass {
		pool.Close()
		return nil, "", fmt.Errorf("backup and restore operator role must bypass RLS to inspect every workspace")
	}
	tx, err := pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err == nil {
		err = s.owner.Fence(ctx, tx, false)
		tx.Rollback()
	}
	if err != nil {
		pool.Close()
		return nil, "", fmt.Errorf("operator DSN must identify the owned PostgreSQL database")
	}
	return pool, dsn, nil
}

func postgresTool(ctx context.Context, tool, dsn string, input io.Reader, output io.Writer, args ...string) error {
	path := os.Getenv("DING_" + strings.ToUpper(tool))
	if path == "" {
		path = tool
	} else if !filepath.IsAbs(path) {
		return fmt.Errorf("PostgreSQL tool override must be an absolute path")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("native PostgreSQL backup tools require a URL DSN reference")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid native PostgreSQL connection reference")
	}
	// Only the database name is in argv; authentication stays in the environment.
	if tool == "pg_restore" {
		args = append(args, "--dbname=dbname='"+strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(cfg.Database)+"'")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	// Do not inherit PGOPTIONS, PGSERVICE or another connection's defaults.
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "PG") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "PGHOST="+cfg.Host, "PGPORT="+strconv.Itoa(int(cfg.Port)), "PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password, "PGDATABASE="+cfg.Database, "PGCONNECT_TIMEOUT=5")
	for key, env := range map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "sslpassword": "PGSSLPASSWORD", "channel_binding": "PGCHANNELBINDING", "target_session_attrs": "PGTARGETSESSIONATTRS"} {
		if value := u.Query().Get(key); value != "" {
			cmd.Env = append(cmd.Env, env+"="+value)
		}
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("PostgreSQL %s failed; verify matching client tools and operator privileges", tool)
	}
	return nil
}

// BackupPostgres holds execution ownership for a consistent native dump of both
// private domain schemas and streams it directly into an encrypted age file.
// The external credential master key is deliberately kept separately.
func BackupPostgres(ctx context.Context, c Config, adminEnv, output, recipient string) error {
	if c.Database.Effective().Backend != "postgres" {
		return fmt.Errorf("PostgreSQL backend required")
	}
	rec, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return fmt.Errorf("valid age X25519 recipient required")
	}
	s, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx = s.Context()
	admin, dsn, err := postgresOperatorPool(ctx, c, adminEnv, s)
	if err != nil {
		return err
	}
	defer admin.Close()
	instance, err := s.Control.InstanceID(ctx)
	if err != nil {
		return err
	}
	out, err := mcpconfig.CreatePrivate(output)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		out.Close()
		if !complete {
			os.Remove(output)
		}
	}()
	encrypted, err := age.Encrypt(out, rec)
	if err != nil {
		return err
	}
	header := postgresBackupHeader{"ding-postgres/v1", state.PostgresSchemaVersion, store.PostgresSchemaVersion, instance, time.Now().UTC()}
	if err = json.NewEncoder(encrypted).Encode(header); err != nil {
		return err
	}
	if err = postgresTool(ctx, "pg_dump", dsn, nil, encrypted, "--format=custom", "--no-owner", "--no-acl", "--schema=ding_control", "--schema=ding_runtime"); err != nil {
		return err
	}
	if err = encrypted.Close(); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
