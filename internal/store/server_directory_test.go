package store

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

func TestPersonalStoreRefusesBoundServerDirectories(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			dir := t.TempDir()
			if err := database.BindDirectory(dir, backend, "instance"); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(context.Background(), dir); err == nil {
				s.Close()
				t.Fatal("personal daemon opened server artifacts")
			}
		})
	}
}
