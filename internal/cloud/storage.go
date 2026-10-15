package cloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/database"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

type postgresWorkspaces struct {
	pool  *sql.DB
	owner *database.Ownership
}

func (p postgresWorkspaces) Open(ctx context.Context, id string) (*store.Store, error) {
	return store.OpenPostgres(ctx, p.pool, id, p.owner)
}
func (p postgresWorkspaces) Delete(ctx context.Context, id string) error {
	return store.DeletePostgresWorkspace(ctx, p.pool, id, p.owner)
}

// ServerStorage owns one selected backend for control and every workspace.
// PostgreSQL has one shared query pool plus one direct ownership connection.
type ServerStorage struct {
	Control    *state.DB
	workspaces workspaceStorage
	pool       *sql.DB
	owner      *database.Ownership
	ctx        context.Context
}

func (s *ServerStorage) Context() context.Context { return s.ctx }
func (s *ServerStorage) ConfigurePool(p *Pool)    { p.storage = s.workspaces }
func (s *ServerStorage) Close() error {
	var closePool error
	if s.pool != nil {
		closePool = s.pool.Close()
	}
	return errors.Join(s.Control.Close(), closePool, s.owner.Close())
}

func OpenServerStorage(ctx context.Context, dir string, c database.Config) (result *ServerStorage, err error) {
	if err = c.Validate(); err != nil {
		return nil, err
	}
	c = c.Effective()
	if err = ValidateServerDirectory(dir); err != nil {
		return nil, err
	}
	if err = database.CheckDirectory(dir, c.Backend); err != nil {
		return nil, err
	}
	s := &ServerStorage{ctx: ctx, workspaces: sqliteWorkspaces{dir}}
	ok := false
	defer func() {
		if !ok {
			if s.Control != nil {
				s.Control.Close()
			}
			if s.pool != nil {
				s.pool.Close()
			}
			s.owner.Close()
		}
	}()
	if c.Backend == "sqlite" {
		s.Control, err = state.Open(ctx, dir)
		if err != nil {
			return nil, err
		}
	} else {
		if _, e := os.Lstat(filepath.Join(dir, "control.db")); !os.IsNotExist(e) {
			return nil, fmt.Errorf("SQLite control state exists; use the explicit backend migration workflow")
		}
		s.pool, err = database.OpenPostgres(ctx, c, nil)
		if err != nil {
			return nil, err
		}
		if err = store.ValidatePostgres(ctx, s.pool); err != nil {
			return nil, err
		}
		if _, err = state.OpenPostgres(ctx, s.pool); err != nil {
			return nil, err
		}
		ownershipConfig := c
		ownershipConfig.MaxOpenConns = 1
		dedicated, e := database.OpenPostgres(ctx, ownershipConfig, nil)
		if e != nil {
			return nil, e
		}
		s.owner, err = database.TakeOwnership(ctx, dedicated)
		if err != nil {
			return nil, err
		}
		s.ctx = s.owner.Context()
		s.Control, err = state.OpenPostgres(s.ctx, s.pool, s.owner)
		if err != nil {
			return nil, err
		}
		s.workspaces = postgresWorkspaces{s.pool, s.owner}
	}
	instance, err := s.Control.InstanceID(s.ctx)
	if err != nil {
		return nil, err
	}
	if err = database.BindDirectory(dir, c.Backend, instance); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}
