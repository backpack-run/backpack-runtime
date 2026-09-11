package integrations

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreservesArgumentsAndChildEnvironment(t *testing.T) {
	if os.Getenv("GO_WANT_INTEGRATION_HELPER") == "1" {
		fmt.Printf("args=%s env=%s", strings.Join(os.Args[1:], "|"), os.Getenv("BACKPACK_TEST_VALUE"))
		os.Exit(0)
	}
	managed, _ := NewArguments("-test.run=TestRunPreservesArgumentsAndChildEnvironment", "--", "managed value")
	passthrough, _ := NewArguments("literal;not-a-shell")
	variable, _ := Set("BACKPACK_TEST_VALUE", "child")
	helperVariable, helperErr := Set("GO_WANT_INTEGRATION_HELPER", "1")
	environment, _ := NewEnvironmentOverlay(variable, mustVariable(t, helperVariable, helperErr))
	invocation := Invocation{Executable: os.Args[0], ManagedArguments: managed, PassthroughArguments: passthrough, Environment: environment, IsolatedConfigDirectory: filepath.Join(t.TempDir(), "config")}
	var output bytes.Buffer
	if err := Run(context.Background(), invocation, ProcessIO{Stdout: &output, Stderr: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "managed value|literal;not-a-shell") || !strings.Contains(output.String(), "env=child") {
		t.Fatalf("unexpected helper output %q", output.String())
	}
}

func mustVariable(t *testing.T, variable Variable, err error) Variable {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return variable
}
