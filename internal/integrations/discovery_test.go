package integrations

import (
	"errors"
	"reflect"
	"testing"
)

func TestDiscoveryUsesDeclaredCandidateOrder(t *testing.T) {
	var lookedUp []string
	discovery := Discovery{LookPath: func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		if name == "agent.exe" {
			return `C:\Tools\agent.exe`, nil
		}
		return "", errors.New("missing")
	}}
	descriptor := testDescriptor("agent")
	installation, err := discovery.Detect(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if installation.Executable != `C:\Tools\agent.exe` || !reflect.DeepEqual(lookedUp, []string{"agent", "agent.exe"}) {
		t.Fatalf("installation=%#v lookups=%v", installation, lookedUp)
	}
}

func TestDiscoveryDoesNotInstallMissingBinary(t *testing.T) {
	discovery := Discovery{LookPath: func(string) (string, error) { return "", errors.New("missing") }}
	_, err := discovery.Detect(testDescriptor("agent"))
	if !errors.Is(err, ErrExecutableNotFound) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestDiscoveryRequiresInjectedLookup(t *testing.T) {
	if _, err := (Discovery{}).Detect(testDescriptor("agent")); err == nil {
		t.Fatal("nil PATH lookup accepted")
	}
}
