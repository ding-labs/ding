package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Run only in the qualification tmpfs, never against the host's ordinary disk.
func TestPhysicalDiskFullRollbackAndRecovery(t *testing.T) {
	root := os.Getenv("DING_FS_FULL_DIR")
	if root == "" {
		t.Skip("requires dedicated 8 MiB qualification tmpfs")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil || fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 16<<20 {
		t.Fatal("requires a dedicated tmpfs of at most 16 MiB", err)
	}
	dir, err := os.MkdirTemp(root, "store-")
	if err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir)
	seed(t, s)
	c, d := fixture(t)
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	filler := filepath.Join(root, "filler")
	f, err := os.Create(filler)
	if err != nil {
		t.Fatal(err)
	}
	var written int64
	block := make([]byte, 65536)
	for written < 16<<20 {
		n, e := f.Write(block)
		written += int64(n)
		if e != nil {
			err = e
			break
		}
	}
	f.Close()
	if !errors.Is(err, syscall.ENOSPC) {
		os.Remove(filler)
		t.Fatal("fixture is not physically full", written, err)
	}
	defer os.Remove(filler)
	if err := s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "physical-full", testNow) }); err == nil {
		t.Fatal("acknowledged input on full filesystem")
	}
	if count(t, s, "observations") != 0 || count(t, s, "events") != 0 || count(t, s, "outbox") != 0 {
		t.Fatal("partially committed input")
	}
	if err := os.Remove(filler); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTest(t, dir)
	if err := reopened.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "recovered", testNow) }); err != nil {
		t.Fatal("failed to recover after space returned", err)
	}
	h, err := reopened.Health(context.Background())
	if err != nil || h.Integrity != "ok" || count(t, reopened, "events") != 1 {
		t.Fatal(h, err)
	}
	t.Logf("actual ENOSPC after %d filler bytes; failed transaction rolled back; reopened store accepted a durable event", written)
}
func TestPrepareReadOnlyFixture(t *testing.T) {
	dir := os.Getenv("DING_FS_PREPARE_DIR")
	if dir == "" {
		t.Skip("qualification fixture setup only")
	}
	s := openTest(t, dir)
	seed(t, s)
	c, d := fixture(t)
	if err := s.Update(context.Background(), func(tx *Tx) error { return transition(tx, c, d, "committed", testNow) }); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestPhysicalReadOnlyStartupPreservesCommittedData(t *testing.T) {
	dir := os.Getenv("DING_FS_READONLY_DIR")
	if dir == "" {
		t.Skip("requires qualification read-only mount")
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte("probe"), 0600); !errors.Is(err, syscall.EROFS) {
		t.Fatal("not a read-only filesystem", err)
	}
	if s, err := Open(context.Background(), dir); err == nil {
		s.Close()
		t.Fatal("started writer on read-only filesystem")
	}
	db, err := sql.Open("sqlite", readOnlyURI(filepath.Join(dir, "ding.db"))+"&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"events", "observations", "outbox"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 1 {
			t.Fatal(table, n, err)
		}
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal(integrity, err)
	}
	t.Log("actual EROFS prevented startup; pre-existing event, observation and outbox intent intact")
}
