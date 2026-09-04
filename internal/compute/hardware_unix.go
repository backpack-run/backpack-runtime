//go:build !windows

package compute

import "context"

func fillPlatformHardware(context.Context, *Hardware) {}
