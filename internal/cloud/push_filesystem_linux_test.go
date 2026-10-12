package cloud

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ding-labs/ding/internal/store"
)

func TestCloudPhysicalDiskReservePreservesReconciliation(t *testing.T) {
	root := os.Getenv("DING_FS_FULL_DIR")
	if root == "" {
		t.Skip("requires dedicated qualification tmpfs")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil || fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 16<<20 {
		t.Fatal("requires bounded dedicated tmpfs", err)
	}
	t.Setenv("TMPDIR", root)
	s, h, sessions, grants := pushSetup(t)
	s.Push.MinFreeBytes = 1 << 20
	body := `{"repo":"test","failed":true}`
	first := receiptData(t, pushRequest(h, grants[0].Token, "first", body))
	filler := filepath.Join(root, "cloud-filler")
	f, err := os.Create(filler)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filler)
	for used := 0; used < 16<<20; {
		n, e := f.Write(make([]byte, 65536))
		used += n
		if e != nil {
			err = e
			break
		}
	}
	f.Close()
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("not physically full", err)
	}
	replay := receiptData(t, pushRequest(h, grants[0].Token, "first", body))
	if !replay.Duplicate || replay.AcceptanceID != first.AcceptanceID {
		t.Fatal("full disk lost receipt", replay)
	}
	if w := pushRequest(h, grants[0].Token, "new", body); w.Code != 503 {
		t.Fatal("full filesystem acknowledged input", w.Code, w.Body.String())
	}
	if err := os.Remove(filler); err != nil {
		t.Fatal(err)
	}
	receiptData(t, pushRequest(h, grants[0].Token, "new", body))
	tenant, _ := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 2 || u.Observations != 2 {
			t.Error("incorrect acceptance accounting", u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	t.Log("ENOSPC closes new cloud acceptance; committed receipt replay survives; capacity recovery accepts exact pending request once")
}
