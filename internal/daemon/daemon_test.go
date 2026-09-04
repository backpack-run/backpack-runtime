package daemon

import (
	"context"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"os"
	"testing"
	"time"
)

func TestStartupLockIsExclusiveAndRecoversStaleFile(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := acquire(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if second, err := acquire(ctx, paths); err == nil {
		second.Close()
		t.Fatal("second startup acquired lock")
	}
	first.Close()
	old := time.Now().Add(-time.Minute)
	if err = os.Chtimes(lockPath(paths), old, old); err != nil {
		t.Fatal(err)
	}
	recovered, err := acquire(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Close()
}
