//go:build windows

package mcpconfig

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openPrivate(path string, create bool) (*os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, ErrPrivate
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrPrivate
	}
	access := uint32(windows.GENERIC_READ | windows.READ_CONTROL)
	disposition := uint32(windows.OPEN_EXISTING)
	var sa *windows.SecurityAttributes
	if create {
		access = windows.GENERIC_WRITE | windows.READ_CONTROL
		disposition = windows.CREATE_NEW
		sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)")
		if err != nil {
			return nil, ErrPrivate
		}
		sa = &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	}
	h, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ, sa, disposition, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	fail := func() (*os.File, error) { f.Close(); return nil, ErrPrivate }
	var file windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &file) != nil || file.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return fail()
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fail()
	}
	owner, _, err := sd.Owner()
	if err != nil || !owner.Equals(user.User.Sid) {
		return fail()
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return fail()
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, (**windows.ACCESS_ALLOWED_ACE)(unsafe.Pointer(&ace))) != nil {
			return fail()
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE && ace.Mask != 0 {
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sid.Equals(user.User.Sid) && sid.String() != "S-1-5-18" && sid.String() != "S-1-5-32-544" {
				return fail()
			}
		} else if ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE && ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fail()
		}
	}
	return f, nil
}
