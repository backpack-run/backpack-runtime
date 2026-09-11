package integrations

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArgumentsPreserveLiteralBoundariesAndCopies(t *testing.T) {
	input := []string{"--model", "name with spaces", `$(not-a-shell)`, "--", "--provider-flag"}
	arguments, err := NewArguments(input...)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "mutated"
	got := arguments.Values()
	got[0] = "also-mutated"
	if arguments.Values()[0] != "--model" {
		t.Fatal("arguments were mutated through caller-owned slices")
	}
	if _, err = NewArguments("bad\x00argument"); err == nil {
		t.Fatal("NUL argument accepted")
	}
}

func TestInvocationKeepsManagedAndPassthroughArgumentsOrdered(t *testing.T) {
	managed, _ := NewArguments("--model", "coder")
	passthrough, _ := NewArguments("--", "--resume", "session with spaces")
	configRoot := filepath.Join(t.TempDir(), "integrations")
	configDirectory, err := IsolatedConfigDirectory(configRoot, "agent")
	if err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{Executable: "agent", ManagedArguments: managed, PassthroughArguments: passthrough, IsolatedConfigDirectory: configDirectory}
	if err = invocation.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "coder", "--", "--resume", "session with spaces"}
	if got := invocation.Args(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInvocationRequiresAbsoluteIsolatedConfig(t *testing.T) {
	if _, err := IsolatedConfigDirectory("relative", "agent"); err == nil {
		t.Fatal("relative integration root accepted")
	}
	if _, err := IsolatedConfigDirectory(t.TempDir(), "../agent"); err == nil {
		t.Fatal("unsafe integration ID accepted")
	}
	if err := (Invocation{Executable: "agent", IsolatedConfigDirectory: "relative"}).Validate(); err == nil {
		t.Fatal("relative invocation config accepted")
	}
}

func TestInvocationFormattingDoesNotLeakEnvironmentSecrets(t *testing.T) {
	token, _ := Secret("AGENT_TOKEN", "do-not-log")
	overlay, _ := NewEnvironmentOverlay(token)
	invocation := Invocation{Executable: "agent", Environment: overlay, IsolatedConfigDirectory: t.TempDir()}
	for _, formatted := range []string{fmt.Sprintf("%v", invocation), fmt.Sprintf("%#v", invocation)} {
		if strings.Contains(formatted, "do-not-log") {
			t.Fatalf("credential leaked through invocation formatting: %q", formatted)
		}
	}
}
