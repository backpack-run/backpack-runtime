//go:build !windows

package compute

import "os"

func attachProcessLifetime(*os.Process) (func(), error) { return func() {}, nil }
