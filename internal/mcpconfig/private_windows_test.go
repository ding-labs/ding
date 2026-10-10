//go:build windows

package mcpconfig

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRejectBroadWindowsACL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private.json")
	f, err := CreatePrivate(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;GR;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if f, err := openPrivate(p, false); err == nil {
		f.Close()
		t.Fatal("Users-readable pairing accepted")
	}
}
