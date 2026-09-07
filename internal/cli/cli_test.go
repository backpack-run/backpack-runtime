package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConventionalVersionFlags(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	for _, argument := range []string{"version", "--version", "-v"} {
		var output bytes.Buffer
		if err := Run(context.Background(), []string{argument}, &output, &output, "v0.1.0-alpha.1"); err != nil {
			t.Fatalf("%s: %v", argument, err)
		}
		if !strings.Contains(output.String(), "backpack v0.1.0-alpha.1") {
			t.Fatalf("%s output %q", argument, output.String())
		}
	}
}
