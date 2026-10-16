package railway

import (
	"fmt"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"io"
	"os"
)

// Railway uses one fixed secret reference so clean exec cannot accidentally
// forward an arbitrary environment variable containing a database password.
func configureDatabase(s *Settings, get func(string) string) error {
	if err := s.Config.Database.Validate(); err != nil {
		return err
	}
	if s.Config.Database.Effective().Backend != "postgres" {
		if get("DING_DATABASE_URL") != "" || get("DING_DATABASE_CA_PEM") != "" {
			return fmt.Errorf("database secrets require explicit PostgreSQL selection")
		}
		return nil
	}
	if s.Config.Database.DSNRef.Env != "DING_DATABASE_URL" {
		return fmt.Errorf("Railway PostgreSQL requires dsnRef.env DING_DATABASE_URL")
	}
	s.DatabaseDSN = get("DING_DATABASE_URL")
	if s.DatabaseDSN == "" || len(s.DatabaseDSN) > 16384 {
		return fmt.Errorf("DING_DATABASE_URL must contain a PostgreSQL DSN of at most 16 KiB")
	}
	s.DatabaseCA = get("DING_DATABASE_CA_PEM")
	if len(s.DatabaseCA) > 65536 {
		return fmt.Errorf("DING_DATABASE_CA_PEM exceeds 64 KiB")
	}
	return nil
}

// Restore only after the unprivileged exec. The password stays out of startup
// JSON and the process's initial environment exposed through /proc.
func restoreDatabaseEnvironment(c database.Config, path string) error {
	if c.Effective().Backend != "postgres" {
		return nil
	}
	if c.DSNRef == nil || c.DSNRef.Env != "DING_DATABASE_URL" {
		return fmt.Errorf("invalid Railway database secret reference")
	}
	f, err := mcpconfig.OpenPrivate(path)
	if err != nil {
		return fmt.Errorf("cannot read private PostgreSQL DSN")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(b) == 0 || len(b) > 16384 {
		return fmt.Errorf("invalid private PostgreSQL DSN")
	}
	return os.Setenv(c.DSNRef.Env, string(b))
}
