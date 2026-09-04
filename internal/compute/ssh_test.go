package compute

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/models"
)

type recordingSSHRunner struct {
	calls   []string
	cached  bool
	inspect []byte
	started []string
}

func (r *recordingSSHRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if strings.Contains(call, "BP_OS=") && r.inspect != nil {
		return r.inspect, nil
	}
	if strings.Contains(call, "BP_HOME=") {
		return []byte("BP_HOME=/home/test\n"), nil
	}
	if strings.Contains(call, "test -f") && r.cached {
		return nil, nil
	}
	if strings.Contains(call, "test -f") {
		return []byte("missing"), errFake{}
	}
	return nil, nil
}
func (r *recordingSSHRunner) Start(name string, args []string, _ io.Writer, _ io.Writer) (Process, error) {
	r.started = append(r.started, name+" "+strings.Join(args, " "))
	return &fakeSSHProcess{}, nil
}

func TestSSHHardwareParsingAndTunnelCommand(t *testing.T) {
	runner := &recordingSSHRunner{inspect: []byte("BP_OS=Ubuntu\nBP_ARCH=x86_64\nBP_CPU=EPYC\nBP_CORES=16\nBP_RAM_KB=33554432\nBP_DISK_KB=104857600\nBP_RUNTIME=yes\nBP_GPU=RTX 6000 Ada, 49140\nBP_CUDA=yes\n")}
	target := NewSSH(SSHConfig{ID: "gpu", Host: "known.example", User: "alice"})
	target.Runner = runner
	h, err := target.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Cores != 16 || h.MemoryTotalGB != 32 || h.DiskAvailableGB != 100 || !h.RuntimeReady || len(h.GPUs) != 1 {
		t.Fatalf("hardware %#v", h)
	}
	_, err = target.Execute(context.Background(), Command{Executable: "llama-server", Args: []string{"--port", "54321", "--model", "/home/a.gguf"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.started) != 1 || !strings.Contains(runner.started[0], "StrictHostKeyChecking=yes") || !strings.Contains(runner.started[0], "127.0.0.1:54321:127.0.0.1:54321") {
		t.Fatalf("command %q", runner.started)
	}
}

type fakeSSHProcess struct{}

func (*fakeSSHProcess) PID() int                   { return 1 }
func (*fakeSSHProcess) Wait() error                { return nil }
func (*fakeSSHProcess) Stop(context.Context) error { return nil }

type errFake struct{}

func (errFake) Error() string { return "exit 1" }

func TestSSHSafeDefaultsAndCachedSync(t *testing.T) {
	runner := &recordingSSHRunner{cached: true}
	target := NewSSH(SSHConfig{ID: "gpu-1", Host: "gpu.example.org", User: "alice"})
	target.Runner = runner
	if err := target.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	model := &models.Installed{ID: "test", Revision: "abc", Directory: t.TempDir(), Package: models.Package{ID: "q4", Filename: "model.gguf", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}
	if err := target.PrepareModel(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "scp ") {
			t.Fatal("cached artifact was copied")
		}
		if strings.Contains(call, "StrictHostKeyChecking=no") {
			t.Fatal("host checking disabled")
		}
	}
	if got := target.ResolvePath(model.Entrypoint()); !strings.HasPrefix(got, "/home/test/.backpack/models/test/abc/q4/") {
		t.Fatalf("remote path %q", got)
	}
}
func TestSSHRejectsUnsafeConfiguration(t *testing.T) {
	for _, config := range []SSHConfig{{ID: "../gpu", Host: "host"}, {ID: "gpu", Host: "host;whoami"}, {ID: "gpu", Host: "host", RemoteRoot: "../../tmp"}} {
		if err := NewSSH(config).validate(); err == nil {
			t.Fatalf("accepted %#v", config)
		}
	}
}

func TestSSHBootstrapsAndVerifiesManagedRuntime(t *testing.T) {
	runner := &recordingSSHRunner{}
	target := NewSSH(SSHConfig{ID: "gpu", Host: "known.example", User: "alice"})
	target.Runner = runner
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "llama-server"), []byte("server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("server"))
	exe, err := target.PrepareRuntime(context.Background(), RuntimeBundle{Engine: "llama.cpp", Version: "b1", Variant: "linux-amd64-cpu", Directory: dir, Executable: "llama-server", Files: []RuntimeFile{{Path: "llama-server", SHA256: fmt.Sprintf("%x", sum)}}})
	if err != nil {
		t.Fatal(err)
	}
	if exe != "/home/test/.backpack/runtimes/llama.cpp/b1/linux-amd64-cpu/llama-server" {
		t.Fatalf("executable %q", exe)
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "sha256sum") || !strings.Contains(joined, ".backpack/runtimes/llama.cpp/b1/linux-amd64-cpu") {
		t.Fatalf("calls:\n%s", joined)
	}
	if strings.Contains(joined, "command -v llama-server") {
		t.Fatal("required global llama-server")
	}
	if !strings.Contains(joined, "scp ") {
		t.Fatal("runtime was not transferred")
	}
}
