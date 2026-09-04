package runtimebundle

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

type memoryFetcher struct {
	mu    sync.Mutex
	data  []byte
	calls int
	fail  error
}

func (f *memoryFetcher) Fetch(_ context.Context, _ string, w io.Writer, _ func(int64)) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	_, err := w.Write(f.data)
	return err
}

type testTarget struct{ hardware compute.Hardware }

func (t testTarget) Name() string                                          { return "local" }
func (t testTarget) Kind() string                                          { return "local" }
func (t testTarget) Inspect(context.Context) (compute.Hardware, error)     { return t.hardware, nil }
func (t testTarget) Prepare(context.Context) error                         { return nil }
func (t testTarget) PrepareModel(context.Context, *models.Installed) error { return nil }
func (t testTarget) ResolvePath(x string) string                           { return x }
func (t testTarget) Execute(context.Context, compute.Command) (compute.Process, error) {
	return nil, fmt.Errorf("unused")
}

func archive(t *testing.T, name, contents string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, contents)
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func testCatalog(data []byte) Catalog {
	sum := sha256.Sum256(data)
	artifact := Artifact{URL: "https://example.test/a.zip", SHA256: fmt.Sprintf("%x", sum), Format: "zip"}
	return Catalog{SchemaVersion: 1, CatalogVersion: "test", Runtimes: []Runtime{{
		Engine: "llama.cpp", Version: "b1", CompatibleRevisions: []string{"commit1"},
		Source: Source{Repository: "https://example.test/src", Revision: "b1"}, License: "MIT",
		Variants: []Variant{
			{ID: "win-cuda", OS: "windows", Architecture: "amd64", Backend: "cuda", Executable: "llama-server.exe", Artifacts: []Artifact{artifact}},
			{ID: "win-vulkan", OS: "windows", Architecture: "amd64", Backend: "vulkan", Executable: "llama-server.exe", Artifacts: []Artifact{artifact}},
			{ID: "win-cpu", OS: "windows", Architecture: "amd64", Backend: "cpu", Executable: "llama-server.exe", Artifacts: []Artifact{artifact}},
		},
	}}}
}

func TestResolveUsesVersionPlatformAndBackendPrecedence(t *testing.T) {
	data := archive(t, "llama-server.exe", "binary")
	m := Manager{Catalog: testCatalog(data)}
	_, v, err := m.Resolve(models.RuntimeRequirement{Engine: "llama.cpp", Version: "commit1"}, compute.Hardware{OS: "Windows 11", Architecture: "x86_64", Backends: []string{"cpu", "vulkan", "cuda"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend != "cuda" {
		t.Fatalf("selected %s", v.Backend)
	}
	_, v, err = m.Resolve(models.RuntimeRequirement{Engine: "llama.cpp", Version: "b1"}, compute.Hardware{OS: "windows", Architecture: "amd64", Backends: []string{"cpu", "vulkan"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend != "vulkan" {
		t.Fatalf("selected %s", v.Backend)
	}
	if _, _, err = m.Resolve(models.RuntimeRequirement{Engine: "llama.cpp", Version: "unknown"}, compute.Hardware{OS: "windows", Architecture: "amd64", Backends: []string{"cpu"}}); err == nil {
		t.Fatal("accepted incompatible version")
	}
}

func TestBuiltinCatalogIsValidAndCarriesLicense(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Runtimes) != 1 || len(c.Runtimes[0].LicenseArtifacts) != 1 {
		t.Fatalf("catalog %#v", c)
	}
}

func TestEnsureIsAtomicConcurrentAndCached(t *testing.T) {
	data := archive(t, "bin/llama-server.exe", "binary")
	fetch := &memoryFetcher{data: data}
	m := &Manager{Paths: config.NewPaths(t.TempDir()), Catalog: testCatalog(data), Fetcher: fetch, locks: map[string]*sync.Mutex{}}
	target := testTarget{compute.Hardware{OS: "windows", Architecture: "amd64", Backends: []string{"cpu"}}}
	req := models.RuntimeRequirement{Engine: "llama.cpp", Version: "b1"}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x, err := m.Ensure(context.Background(), req, target)
			if err == nil {
				_, err = os.Stat(x.Executable)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if fetch.calls != 1 {
		t.Fatalf("downloads=%d", fetch.calls)
	}
	items, err := m.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if err = m.Verify(items[0]); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRejectsChecksumMismatchAndCleansStage(t *testing.T) {
	data := archive(t, "llama-server.exe", "binary")
	c := testCatalog(data)
	c.Runtimes[0].Variants[2].Artifacts[0].SHA256 = strings.Repeat("0", 64)
	root := t.TempDir()
	m := &Manager{Paths: config.NewPaths(root), Catalog: c, Fetcher: &memoryFetcher{data: data}, locks: map[string]*sync.Mutex{}}
	_, err := m.Ensure(context.Background(), models.RuntimeRequirement{Engine: "llama.cpp", Version: "b1"}, testTarget{compute.Hardware{OS: "windows", Architecture: "amd64", Backends: []string{"cpu"}}})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error=%v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "runtimes", "llama.cpp", "b1", ".*-install-*"))
	if len(matches) != 0 {
		t.Fatalf("staging remains: %v", matches)
	}
}

func TestArchiveTraversalIsRejected(t *testing.T) {
	data := archive(t, "../escape.exe", "bad")
	path := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	err := extractZip(path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("error=%v", err)
	}
}

func TestRemoveRejectsBroadPathComponents(t *testing.T) {
	m := Manager{Paths: config.NewPaths(t.TempDir())}
	if err := m.Remove(".", ".", "."); err == nil {
		t.Fatal("accepted broad removal target")
	}
}
