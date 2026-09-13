//go:build windows

package daemon

import "os"

// Windows relies on the per-user AppData directory ACL. Go permission bits do
// not represent the Windows DACL, so applying a POSIX mode check is incorrect.
func validateStatePermissions(os.FileInfo) error { return nil }
