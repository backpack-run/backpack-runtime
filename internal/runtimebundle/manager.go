package runtimebundle

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

type Installed struct {
	SchemaVersion int               `json:"schema_version"`
	Engine        string            `json:"engine"`
	Version       string            `json:"version"`
	Variant       string            `json:"variant"`
	OS            string            `json:"os"`
	Architecture  string            `json:"architecture"`
	Backend       string            `json:"backend"`
	Executable    string            `json:"executable"`
	Files         map[string]string `json:"files"`
	Source        Source            `json:"source"`
	License       string            `json:"license"`
	Directory     string            `json:"-"`
}

type Fetcher interface {
	Fetch(context.Context, string, io.Writer, func(int64)) error
}
type HTTPFetcher struct{ Client *http.Client }

func (f HTTPFetcher) Fetch(ctx context.Context, url string, dst io.Writer, progress func(int64)) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("runtime downloads require HTTPS: %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Minute}
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("download returned %s", res.Status)
	}
	_, err = io.Copy(dst, &progressReader{r: res.Body, fn: progress})
	return err
}

type progressReader struct {
	r      io.Reader
	fn     func(int64)
	n      int64
	last   int64
	lastAt time.Time
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, e := r.r.Read(p)
	r.n += int64(n)
	if r.fn != nil && (r.n-r.last >= 1<<20 || e == io.EOF || time.Since(r.lastAt) >= 500*time.Millisecond) {
		r.fn(r.n)
		r.last, r.lastAt = r.n, time.Now()
	}
	return n, e
}

type Manager struct {
	Paths   config.Paths
	Catalog Catalog
	Fetcher Fetcher
	Sink    events.Sink
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
}

func New(paths config.Paths, sink events.Sink) (*Manager, error) {
	c, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	return &Manager{Paths: paths, Catalog: c, Fetcher: HTTPFetcher{}, Sink: sink, locks: map[string]*sync.Mutex{}}, nil
}

func (m *Manager) Resolve(req models.RuntimeRequirement, h compute.Hardware) (Runtime, Variant, error) {
	var selected Runtime
	for _, r := range m.Catalog.Runtimes {
		if !strings.EqualFold(r.Engine, req.Engine) {
			continue
		}
		if req.Version == "" || req.Version == r.Version || containsFold(r.CompatibleRevisions, req.Version) {
			selected = r
			break
		}
	}
	if selected.Engine == "" {
		return Runtime{}, Variant{}, fmt.Errorf("no trusted runtime bundle satisfies %s %q", req.Engine, req.Version)
	}
	osName := normalizeOS(h.OS)
	arch := normalizeArch(h.Architecture)
	preferred := []string{}
	if osName == "darwin" {
		if has(h.Backends, "metal") {
			preferred = append(preferred, "metal")
		}
	} else {
		if has(h.Backends, "cuda") {
			preferred = append(preferred, "cuda")
		}
		if has(h.Backends, "vulkan") {
			preferred = append(preferred, "vulkan")
		}
	}
	preferred = append(preferred, "cpu")
	for _, backend := range preferred {
		for _, v := range selected.Variants {
			if v.OS == osName && v.Architecture == arch && v.Backend == backend {
				return selected, v, nil
			}
		}
	}
	return Runtime{}, Variant{}, fmt.Errorf("no %s runtime bundle supports %s/%s with backends %v", req.Engine, osName, arch, h.Backends)
}

func (m *Manager) Ensure(ctx context.Context, req models.RuntimeRequirement, target compute.Target) (*Installed, error) {
	h, err := target.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect compute for runtime selection: %w", err)
	}
	r, v, err := m.Resolve(req, h)
	if err != nil {
		return nil, err
	}
	key := r.Engine + "/" + r.Version + "/" + v.ID
	lock := m.lock(key)
	lock.Lock()
	defer lock.Unlock()
	if found, err := m.load(r, v); err == nil {
		return m.ensureRemote(ctx, target, found)
	}
	release, err := acquireInstallLock(ctx, m.directory(r, v)+".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	if found, err := m.load(r, v); err == nil {
		return m.ensureRemote(ctx, target, found)
	}
	events.Emit(m.Sink, events.Event{Type: events.RuntimeDownloadStarted, Kind: events.Status, Message: fmt.Sprintf("Installing %s %s (%s)", r.Engine, r.Version, v.ID)})
	installed, err := m.install(ctx, r, v)
	if err != nil {
		return nil, fmt.Errorf("Backpack requires %s %s; the runtime is not installed and could not be downloaded: %w; retry when online or run `backpack runtime install %s`", r.Engine, r.Version, err, r.Engine)
	}
	events.Emit(m.Sink, events.Event{Type: events.RuntimeInstalled, Kind: events.Complete, Message: fmt.Sprintf("Installed %s %s (%s)", r.Engine, r.Version, v.ID)})
	return m.ensureRemote(ctx, target, installed)
}

func (m *Manager) ensureRemote(ctx context.Context, target compute.Target, in *Installed) (*Installed, error) {
	remote, ok := target.(interface {
		PrepareRuntime(context.Context, compute.RuntimeBundle) (string, error)
	})
	if !ok || target.Kind() == "local" {
		local := *in
		local.Executable = filepath.Join(in.Directory, filepath.FromSlash(in.Executable))
		return &local, nil
	}
	files := make([]compute.RuntimeFile, 0, len(in.Files))
	for name, sum := range in.Files {
		files = append(files, compute.RuntimeFile{Path: name, SHA256: sum})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	exe, err := remote.PrepareRuntime(ctx, compute.RuntimeBundle{Engine: in.Engine, Version: in.Version, Variant: in.Variant, Directory: in.Directory, Executable: in.Executable, Files: files})
	if err != nil {
		return nil, err
	}
	copy := *in
	copy.Executable = exe
	return &copy, nil
}

func (m *Manager) List() ([]Installed, error) {
	var out []Installed
	_ = filepath.Walk(m.Paths.Runtimes, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && info.Name() == "manifest.json" {
			b, e := os.ReadFile(path)
			var x Installed
			if e == nil && json.Unmarshal(b, &x) == nil {
				x.Directory = filepath.Dir(path)
				out = append(out, x)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].Engine+out[i].Version+out[i].Variant < out[j].Engine+out[j].Version+out[j].Variant
	})
	return out, nil
}
func (m *Manager) Verify(in Installed) error { _, err := m.verify(in); return err }
func (m *Manager) Remove(engine, version, variant string) error {
	if !validComponent(engine) || !validComponent(version) || !validComponent(variant) {
		return fmt.Errorf("engine, version, and variant are required")
	}
	items, err := m.List()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Engine == engine && item.Version == version && item.Variant == variant {
			return os.RemoveAll(item.Directory)
		}
	}
	return fmt.Errorf("runtime %s %s (%s) is not installed", engine, version, variant)
}

func (m *Manager) load(r Runtime, v Variant) (*Installed, error) {
	dir := m.directory(r, v)
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var in Installed
	if err = json.Unmarshal(b, &in); err != nil {
		return nil, err
	}
	in.Directory = dir
	if _, err = m.verify(in); err != nil {
		return nil, err
	}
	return &in, nil
}
func (m *Manager) verify(in Installed) (string, error) {
	if in.SchemaVersion != 1 || in.Engine == "" || in.Version == "" || in.Variant == "" {
		return "", fmt.Errorf("invalid runtime manifest")
	}
	exe := filepath.Join(in.Directory, filepath.FromSlash(in.Executable))
	for name, want := range in.Files {
		path := filepath.Join(in.Directory, filepath.FromSlash(name))
		if !within(in.Directory, path) {
			return "", fmt.Errorf("runtime file escapes installation")
		}
		got, err := hashFile(path)
		if err != nil || !strings.EqualFold(got, want) {
			return "", fmt.Errorf("runtime integrity check failed for %s", name)
		}
	}
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("runtime executable: %w", err)
	}
	return exe, nil
}

func (m *Manager) install(ctx context.Context, r Runtime, v Variant) (*Installed, error) {
	if err := m.Paths.Ensure(); err != nil {
		return nil, err
	}
	final := m.directory(r, v)
	parent := filepath.Dir(final)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(parent, "."+safe(v.ID)+"-install-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	artifacts := append(append([]Artifact{}, v.Artifacts...), r.LicenseArtifacts...)
	for i, a := range artifacts {
		archive := filepath.Join(m.Paths.Cache, fmt.Sprintf("runtime-%s-%s-%d", safe(r.Version), safe(v.ID), i))
		f, err := os.OpenFile(archive+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		err = m.Fetcher.Fetch(ctx, a.URL, f, func(n int64) {
			events.Emit(m.Sink, events.Event{Type: events.RuntimeDownloadProgress, Kind: events.Progress, Message: filepath.Base(a.URL), Current: n})
		})
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		got, err := hashFile(archive + ".part")
		if err != nil || !strings.EqualFold(got, a.SHA256) {
			return nil, fmt.Errorf("checksum mismatch for %s", filepath.Base(a.URL))
		}
		if err = os.Rename(archive+".part", archive); err != nil {
			return nil, err
		}
		if err = extract(archive, a.Format, a.Path, stage); err != nil {
			return nil, err
		}
		_ = os.Remove(archive)
	}
	exeRel, err := findExecutable(stage, v.Executable)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	err = filepath.Walk(stage, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(stage, path)
		sum, e := hashFile(path)
		if e == nil {
			files[filepath.ToSlash(rel)] = sum
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	in := Installed{SchemaVersion: 1, Engine: r.Engine, Version: r.Version, Variant: v.ID, OS: v.OS, Architecture: v.Architecture, Backend: v.Backend, Executable: exeRel, Files: files, Source: r.Source, License: r.License, Directory: stage}
	b, _ := json.MarshalIndent(in, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "manifest.json"), b, 0600); err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(final); statErr == nil {
		if existing, e := m.load(r, v); e == nil {
			return existing, nil
		}
		if e := os.Rename(final, final+".invalid-"+fmt.Sprint(time.Now().Unix())); e != nil {
			return nil, e
		}
	}
	if err = os.Rename(stage, final); err != nil {
		return nil, err
	}
	in.Directory = final
	return &in, nil
}

func (m *Manager) directory(r Runtime, v Variant) string {
	return filepath.Join(m.Paths.Runtimes, safe(r.Engine), safe(r.Version), safe(v.ID))
}
func (m *Manager) lock(k string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks[k] == nil {
		m.locks[k] = &sync.Mutex{}
	}
	return m.locks[k]
}

func acquireInstallLock(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	for {
		err := os.Mkdir(path, 0700)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire runtime install lock: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > 2*time.Hour {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for runtime install lock: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func normalizeOS(v string) string {
	v = strings.ToLower(v)
	if strings.Contains(v, "windows") {
		return "windows"
	}
	if strings.Contains(v, "darwin") || strings.Contains(v, "macos") {
		return "darwin"
	}
	if strings.Contains(v, "linux") || strings.Contains(v, "ubuntu") {
		return "linux"
	}
	return v
}
func normalizeArch(v string) string {
	switch strings.ToLower(v) {
	case "x86_64", "x64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return strings.ToLower(v)
}
func has(xs []string, x string) bool {
	for _, v := range xs {
		if strings.EqualFold(v, x) {
			return true
		}
	}
	return false
}
func containsFold(xs []string, x string) bool { return has(xs, x) }
func safe(v string) string                    { return strings.NewReplacer("/", "-", "\\", "-", "..", "-").Replace(v) }
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func findExecutable(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), name) && found == "" {
			rel, _ := filepath.Rel(root, path)
			found = filepath.ToSlash(rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("runtime archive has no %s", name)
	}
	return found, nil
}

func extract(path, format, artifactPath, dst string) error {
	switch format {
	case "zip":
		return extractZip(path, dst)
	case "tar.gz":
		return extractTar(path, dst)
	case "file":
		target, err := safeArchivePath(dst, artifactPath)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, src)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	default:
		return fmt.Errorf("unsupported runtime archive %q", format)
	}
}
func safeArchivePath(dst, name string) (string, error) {
	name = filepath.FromSlash(name)
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive contains absolute path %q", name)
	}
	target := filepath.Join(dst, name)
	if !within(dst, target) {
		return "", fmt.Errorf("archive path escapes destination: %q", name)
	}
	return target, nil
}
func extractZip(path, dst string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > 2000 {
		return fmt.Errorf("runtime archive has too many files")
	}
	for _, f := range z.File {
		target, err := safeArchivePath(dst, f.Name)
		if err != nil {
			return err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime archive contains symlink %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		src, e := f.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode().Perm()|0500)
		if e == nil {
			_, e = io.Copy(out, io.LimitReader(src, 4<<30))
			_ = out.Close()
		}
		_ = src.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func extractTar(path, dst string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	type pendingSymlink struct{ target, linkname string }
	var symlinks []pendingSymlink
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		count++
		if count > 2000 {
			return fmt.Errorf("runtime archive has too many files")
		}
		target, e := safeArchivePath(dst, h.Name)
		if e != nil {
			return e
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(target, 0700); e != nil {
				return e
			}
		case tar.TypeReg:
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0777|0500)
			if e == nil {
				_, e = io.Copy(out, io.LimitReader(tr, 4<<30))
				_ = out.Close()
			}
			if e != nil {
				return e
			}
		case tar.TypeSymlink:
			linkname := filepath.FromSlash(h.Linkname)
			if linkname == "" || filepath.IsAbs(linkname) {
				return fmt.Errorf("runtime archive contains unsafe symlink %q -> %q", h.Name, h.Linkname)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkname))
			if !within(dst, resolved) {
				return fmt.Errorf("runtime archive symlink escapes destination: %q -> %q", h.Name, h.Linkname)
			}
			symlinks = append(symlinks, pendingSymlink{target: target, linkname: linkname})
		default:
			return fmt.Errorf("runtime archive contains unsupported link/device %q", h.Name)
		}
	}
	// Links are created only after regular files so they cannot redirect writes
	// from later archive entries. Only relative, archive-internal, non-dangling
	// symlinks are accepted; hard links and devices remain rejected.
	for _, link := range symlinks {
		resolved := filepath.Clean(filepath.Join(filepath.Dir(link.target), link.linkname))
		if _, err = os.Stat(resolved); err != nil {
			return fmt.Errorf("runtime archive contains dangling symlink %q: %w", link.target, err)
		}
		if err = os.MkdirAll(filepath.Dir(link.target), 0700); err != nil {
			return err
		}
		if err = os.Symlink(link.linkname, link.target); err != nil {
			return fmt.Errorf("create runtime symlink %q: %w", link.target, err)
		}
	}
	return nil
}
