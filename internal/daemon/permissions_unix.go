//go:build !windows

package daemon

import (
	"fmt"
	"os"
)

func validateStatePermissions(info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("runtime state file permissions are too broad; require 0600")
	}
	return nil
}
