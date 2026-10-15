package cloud

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/store"
)

type workspaceStorage interface {
	Open(context.Context, string) (*store.Store, error)
	Delete(context.Context, string) error
}
type sqliteWorkspaces struct{ root string }

func (s sqliteWorkspaces) Open(ctx context.Context, id string) (*store.Store, error) {
	return store.Open(ctx, filepath.Join(s.root, "workspaces", id))
}
func (s sqliteWorkspaces) Delete(ctx context.Context, id string) error {
	path := filepath.Join(s.root, "workspaces", id)
	if _, err := os.Stat(path); err == nil {
		owned, err := store.Open(ctx, path)
		if err != nil {
			return err
		}
		if err = owned.Close(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(path)
}
func (p *Pool) workspaceStorage() workspaceStorage {
	if p.storage != nil {
		return p.storage
	}
	return sqliteWorkspaces{p.root}
}
