package main

import (
	"context"
	"fmt"
	"os"

	"github.com/backpack-run/backpack-runtime/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, version); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
