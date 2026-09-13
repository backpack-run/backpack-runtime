//go:build windows

package cloud

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsStorage = "windows-dpapi"

type dataBlob struct {
	size uint32
	data *byte
}

var (
	crypt32            = windows.NewLazySystemDLL("crypt32.dll")
	cryptProtectData   = crypt32.NewProc("CryptProtectData")
	cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
)

func protectSecret(plain []byte) ([]byte, string, error) {
	result, err := cryptData(cryptProtectData, plain)
	return result, windowsStorage, err
}

func unprotectSecret(protected []byte, storage string) ([]byte, error) {
	if storage != windowsStorage {
		return nil, fmt.Errorf("unsupported Windows credential storage %q", storage)
	}
	return cryptData(cryptUnprotectData, protected)
}

func cryptData(procedure *windows.LazyProc, input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("credential data is empty")
	}
	in := dataBlob{size: uint32(len(input)), data: &input[0]}
	var out dataBlob
	result, _, callErr := procedure.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		uintptr(1), // CRYPTPROTECT_UI_FORBIDDEN
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(input)
	if result == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return nil, fmt.Errorf("Windows DPAPI operation failed: %w", callErr)
		}
		return nil, fmt.Errorf("Windows DPAPI operation failed")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.data))))
	value := append([]byte(nil), unsafe.Slice(out.data, int(out.size))...)
	return value, nil
}

func validateSecretPermissions(os.FileInfo) error { return nil }
