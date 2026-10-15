// Package database owns server storage selection and connection policy.
package database

import (
	"fmt"
	"os"
	"regexp"
)

// SecretRef deliberately supports environment references only. Database passwords
// never belong in server JSON, command arguments, or diagnostic output.
type SecretRef struct {
	Env string `json:"env"`
}

type Config struct {
	Backend      string     `json:"backend,omitempty"`
	DSNRef       *SecretRef `json:"dsnRef,omitempty"`
	MaxOpenConns int        `json:"maxOpenConns,omitempty"`
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (c Config) Effective() Config {
	if c.Backend == "" {
		c.Backend = "sqlite"
	}
	if c.Backend == "postgres" && c.MaxOpenConns == 0 {
		c.MaxOpenConns = 8
	}
	return c
}

func (c Config) Validate() error {
	c = c.Effective()
	switch c.Backend {
	case "sqlite":
		if c.DSNRef != nil || c.MaxOpenConns != 0 {
			return fmt.Errorf("SQLite does not accept dsnRef or maxOpenConns")
		}
	case "postgres":
		if c.DSNRef == nil || !environmentName.MatchString(c.DSNRef.Env) {
			return fmt.Errorf("PostgreSQL requires dsnRef.env naming an environment variable")
		}
		if c.MaxOpenConns < 1 || c.MaxOpenConns > 64 {
			return fmt.Errorf("PostgreSQL maxOpenConns must be between 1 and 64")
		}
	default:
		return fmt.Errorf("database backend must be sqlite or postgres")
	}
	return nil
}

// ResolveDSN never includes the value or variable name in errors. Selection is
// driven only by Config; an environment variable cannot switch a SQLite server.
func (c Config) ResolveDSN(lookup func(string) (string, bool)) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	if c.Effective().Backend != "postgres" {
		return "", nil
	}
	if lookup == nil {
		lookup = os.LookupEnv
	}
	value, ok := lookup(c.DSNRef.Env)
	if !ok || value == "" {
		return "", fmt.Errorf("PostgreSQL DSN environment reference is unset")
	}
	return value, nil
}
