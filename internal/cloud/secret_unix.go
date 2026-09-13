//go:build !windows

package cloud

import (
	"fmt"
	"os"
)

const fileStorage = "restricted-file"

func protectSecret(plain []byte) ([]byte, string, error) {
	return append([]byte(nil), plain...), fileStorage, nil
}

func unprotectSecret(protected []byte, storage string) ([]byte, error) {
	if storage != fileStorage {
		return nil, fmt.Errorf("unsupported credential storage %q", storage)
	}
	return append([]byte(nil), protected...), nil
}

func validateSecretPermissions(info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("cloud credential file permissions are too broad; require 0600")
	}
	return nil
}
