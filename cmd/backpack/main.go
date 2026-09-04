package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/backpack-run/backpack-runtime/internal/cli"
)

var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	build := fmt.Sprintf("%s (commit %s, built %s, %s)", version, commit, buildDate, runtime.Version())
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, build); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
