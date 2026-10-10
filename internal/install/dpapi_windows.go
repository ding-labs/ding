//go:build windows

package install

import (
	"crypto/sha256"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"unsafe"
)

func protectCredential(dir, name string, value []byte, decrypt bool) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("empty protected credential")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if real, e := filepath.EvalSymlinks(absolute); e == nil {
		absolute = real
	}
	entropy := sha256.Sum256([]byte("ding.dpapi.v1/" + absolute + "/" + name))
	input := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	extra := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var output windows.DataBlob
	if decrypt {
		err = windows.CryptUnprotectData(&input, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	} else {
		err = windows.CryptProtectData(&input, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	}
	if err != nil {
		return nil, fmt.Errorf("protected credential unavailable for this Windows user or installation")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	if output.Size == 0 || output.Size > 128<<10 {
		return nil, fmt.Errorf("invalid protected credential size")
	}
	raw := unsafe.Slice(output.Data, int(output.Size))
	copy := append([]byte(nil), raw...)
	clear(raw)
	return copy, nil
}
