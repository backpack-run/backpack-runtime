package runtimebundle

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	if len(c.Runtimes) < 2 {
		t.Fatalf("expected multiple native runtimes, got %#v", c)
	}
	for _, runtime := range c.Runtimes {
		if runtime.License == "" {
			t.Fatalf("runtime %q does not declare a license", runtime.Engine)
		}
	}
}

func TestBuiltinWhisperRuntimeResolvesOnlyPublishedPlatform(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Catalog: c}
	requirement := models.RuntimeRequirement{Engine: "whisper.cpp", Version: "eacbd8234c6654cdbf2c377f72b2106875479bdc"}
	_, variant, err := m.Resolve(requirement, compute.Hardware{OS: "windows", Architecture: "amd64", Backends: []string{"cpu"}})
	if err != nil || variant.ID != "windows-amd64-cpu" {
		t.Fatalf("variant %#v error %v", variant, err)
	}
	if _, _, err = m.Resolve(requirement, compute.Hardware{OS: "linux", Architecture: "amd64", Backends: []string{"cpu"}}); err == nil {
		t.Fatal("accepted unpublished Linux Whisper runtime")
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

func TestListIgnoresQuarantinedAndStagingBundles(t *testing.T) {
	root := t.TempDir()
	m := &Manager{Paths: config.NewPaths(root)}
	installed := Installed{SchemaVersion: 1, Engine: "llama.cpp", Version: "b1", Variant: "win-cpu"}
	manifest, err := json.Marshal(installed)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(m.Paths.Runtimes, "llama.cpp", "b1")
	for _, directory := range []string{
		filepath.Join(base, "win-cpu"),
		filepath.Join(base, "win-cpu.invalid-123"),
		filepath.Join(base, ".win-cpu-install-456"),
	} {
		if err = os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || filepath.Base(items[0].Directory) != "win-cpu" {
		t.Fatalf("listed internal runtime directories: %#v", items)
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

func tarArchive(t *testing.T, entries []tar.Header) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	w := tar.NewWriter(gz)
	for i := range entries {
		header := entries[i]
		if err = w.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			_, _ = io.WriteString(w, "binary")
		}
	}
	if err = w.Close(); err == nil {
		err = gz.Close()
	}
	if err == nil {
		err = file.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTarAllowsOnlyConfinedNonDanglingSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed tar runtimes are Linux/macOS variants; Windows test users cannot reliably create symlinks")
	}
	archivePath := tarArchive(t, []tar.Header{
		{Name: "runtime/libexample.so.1", Mode: 0755, Size: 6, Typeflag: tar.TypeReg},
		{Name: "runtime/libexample.so", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "libexample.so.0"},
		{Name: "runtime/libexample.so.0", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "libexample.so.1"},
	})
	destination := t.TempDir()
	if err := extractTar(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "runtime", "libexample.so"))
	if err != nil || string(data) != "binary" {
		t.Fatalf("symlink content=%q error=%v", data, err)
	}

	for name, linkname := range map[string]string{"escape": "../../outside", "absolute": "/outside", "dangling": "missing.so"} {
		t.Run(name, func(t *testing.T) {
			bad := tarArchive(t, []tar.Header{{Name: "runtime/link.so", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: linkname}})
			if err := extractTar(bad, t.TempDir()); err == nil {
				t.Fatalf("accepted symlink target %q", linkname)
			}
		})
	}
}

func TestRemoveRejectsBroadPathComponents(t *testing.T) {
	m := Manager{Paths: config.NewPaths(t.TempDir())}
	if err := m.Remove(".", ".", "."); err == nil {
		t.Fatal("accepted broad removal target")
	}
}
