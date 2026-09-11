package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/statemigrate"
)

type installedRecord struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Package    string `json:"package"`
}

type Installed struct {
	ID, Repository, Revision, Directory string
	Manifest                            *Manifest
	Package                             Package
	Runtime                             RuntimeRequirement
}
type Manager struct {
	Paths   config.Paths
	Client  *http.Client
	BaseURL string
}

func NewManager(paths config.Paths) *Manager {
	return &Manager{Paths: paths, Client: &http.Client{Timeout: 0}, BaseURL: "https://huggingface.co"}
}

func (m *Manager) Pull(ctx context.Context, entry catalog.Model, sink events.Sink) (*Installed, error) {
	if err := m.Paths.Ensure(); err != nil {
		return nil, err
	}
	events.Emit(sink, events.Event{Type: events.ModelResolve, Kind: events.Status, Message: "Resolving package manifest"})
	resolved, manifestBytes, err := m.resolvePackage(ctx, entry)
	if err != nil {
		return nil, err
	}
	manifest, pkg, dir := resolved.Manifest, resolved.Package, resolved.Directory
	for _, artifact := range pkg.RequiredFiles() {
		destination := filepath.Join(dir, filepath.FromSlash(artifact.Filename))
		if !within(dir, destination) {
			return nil, fmt.Errorf("unsafe artifact path %q", artifact.Filename)
		}
		if ok, _ := verifyFile(destination, artifact.SHA256); ok {
			events.Emit(sink, events.Event{Kind: events.Status, Message: "Verified cached " + artifact.Filename})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return nil, err
		}
		if err := m.downloadVerified(ctx, hfURL(m.BaseURL, entry.Repository, entry.Revision, artifact.Filename), destination, artifact, sink); err != nil {
			return nil, err
		}
	}
	if err := atomicWrite(filepath.Join(dir, "backpack-model.yaml"), manifestBytes); err != nil {
		return nil, err
	}
	record := installedRecord{Repository: entry.Repository, Revision: entry.Revision, Package: pkg.ID}
	if err := statemigrate.WriteRecord(filepath.Join(m.Paths.Manifests, entry.ID+".json"), record); err != nil {
		return nil, fmt.Errorf("write installed model state: %w", err)
	}
	events.Emit(sink, events.Event{Type: events.ModelVerifyComplete, Kind: events.Complete, Message: "Package installed and verified"})
	return &Installed{entry.ID, entry.Repository, entry.Revision, dir, manifest, pkg, manifest.RuntimeFor(pkg)}, nil
}

// ResolvePackage retrieves and validates only trusted metadata. It is used for
// model-fit refusal before multi-gigabyte artifacts are transferred.
func (m *Manager) ResolvePackage(ctx context.Context, entry catalog.Model) (*Installed, error) {
	resolved, _, err := m.resolvePackage(ctx, entry)
	return resolved, err
}

func (m *Manager) resolvePackage(ctx context.Context, entry catalog.Model) (*Installed, []byte, error) {
	manifestURL := hfURL(m.BaseURL, entry.Repository, entry.Revision, "backpack-model.yaml")
	manifestBytes, err := m.get(ctx, manifestURL)
	if err != nil {
		return nil, nil, fmt.Errorf("download manifest: %w", err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, nil, err
	}
	if !acceptedManifestID(entry, manifest.Model.ID) {
		return nil, nil, fmt.Errorf("catalog expected model %q but manifest contains %q", entry.ID, manifest.Model.ID)
	}
	pkg, err := preferredPackage(manifest.Packages)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(m.Paths.Models, entry.ID, entry.Revision, pkg.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create model directory: %w", err)
	}
	return &Installed{entry.ID, entry.Repository, entry.Revision, dir, manifest, pkg, manifest.RuntimeFor(pkg)}, manifestBytes, nil
}

func acceptedManifestID(entry catalog.Model, id string) bool {
	if strings.EqualFold(entry.ID, id) {
		return true
	}
	for _, accepted := range entry.ManifestIDs {
		if strings.EqualFold(accepted, id) {
			return true
		}
	}
	return false
}

func (m *Manager) Installed(id string) (*Installed, error) {
	statePath := filepath.Join(m.Paths.Manifests, id+".json")
	r, err := statemigrate.ReadRecord[installedRecord](statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("model %q is not installed; run `backpack pull %s`", id, id)
		}
		return nil, fmt.Errorf("read installed model state: %w", err)
	}
	dir := filepath.Join(m.Paths.Models, id, r.Revision, r.Package)
	mb, err := os.ReadFile(filepath.Join(dir, "backpack-model.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read installed manifest: %w", err)
	}
	manifest, err := ParseManifest(mb)
	if err != nil {
		return nil, err
	}
	for _, p := range manifest.Packages {
		if p.ID == r.Package {
			return &Installed{id, r.Repository, r.Revision, dir, manifest, p, manifest.RuntimeFor(p)}, nil
		}
	}
	return nil, fmt.Errorf("installed package %q is absent from manifest", r.Package)
}

func (m *Manager) List() ([]*Installed, error) {
	entries, err := os.ReadDir(m.Paths.Manifests)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Installed
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		x, err := m.Installed(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil {
			out = append(out, x)
		}
	}
	return out, nil
}
func (i Installed) Entrypoint() string {
	return filepath.Join(i.Directory, filepath.FromSlash(i.Package.RuntimeEntrypoint()))
}

func (i Installed) ArtifactPath(role string) (string, bool) {
	artifact, ok := i.Package.ArtifactByRole(role)
	if !ok {
		return "", false
	}
	return filepath.Join(i.Directory, filepath.FromSlash(artifact.Filename)), true
}

// Verify checks the complete package, including every split shard and auxiliary
// artifact. This is intentionally callable before every launch so corruption
// cannot be hidden by a previously written installation record.
func (m *Manager) Verify(installed *Installed) error {
	for _, artifact := range installed.Package.RequiredFiles() {
		path := filepath.Join(installed.Directory, filepath.FromSlash(artifact.Filename))
		if !within(installed.Directory, path) {
			return fmt.Errorf("unsafe installed artifact path %q", artifact.Filename)
		}
		ok, err := verifyFile(path, artifact.SHA256)
		if err != nil {
			return fmt.Errorf("verify %s: %w", artifact.Filename, err)
		}
		if !ok {
			return fmt.Errorf("checksum mismatch for installed artifact %s", artifact.Filename)
		}
	}
	return nil
}

func preferredPackage(packages []Package) (Package, error) {
	for _, p := range packages {
		if strings.EqualFold(p.Precision, "Q4_K_M") {
			return p, nil
		}
	}
	if len(packages) > 0 {
		return packages[0], nil
	}
	return Package{}, fmt.Errorf("package has no downloadable artifacts")
}
func hfURL(base, repo, revision, name string) string {
	parts := strings.Split(strings.ReplaceAll(name, "\\", "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.TrimRight(base, "/") + "/" + repo + "/resolve/" + revision + "/" + strings.Join(parts, "/") + "?download=true"
}
func (m *Manager) get(ctx context.Context, address string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	req.Header.Set("User-Agent", "Backpack-Runtime")
	res, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s returned %s", address, res.Status)
	}
	return io.ReadAll(res.Body)
}
func (m *Manager) downloadVerified(ctx context.Context, address, destination string, file File, sink events.Sink) error {
	tmp := destination + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	req.Header.Set("User-Agent", "Backpack-Runtime")
	res, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("download %s: %s", file.Filename, res.Status)
	}
	total := res.ContentLength
	if total <= 0 {
		total = file.SizeBytes
	}
	h := sha256.New()
	reader := &progressReader{r: res.Body, total: total, sink: sink, label: file.Filename, last: time.Now()}
	if _, err = io.Copy(io.MultiWriter(out, h), reader); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, file.SHA256) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", file.Filename, file.SHA256, actual)
	}
	if err = os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace corrupt artifact %s: %w", file.Filename, err)
	}
	if err = os.Rename(tmp, destination); err != nil {
		return fmt.Errorf("install %s: %w", file.Filename, err)
	}
	return nil
}

type progressReader struct {
	r              io.Reader
	current, total int64
	sink           events.Sink
	label          string
	last           time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.current += int64(n)
	if time.Since(p.last) > 200*time.Millisecond || err == io.EOF {
		pct := float64(0)
		if p.total > 0 {
			pct = 100 * float64(p.current) / float64(p.total)
		}
		events.Emit(p.sink, events.Event{Type: events.ModelDownloadProgress, Kind: events.Progress, Message: p.label, Current: p.current, Total: p.total, Percentage: pct})
		p.last = time.Now()
	}
	return n, err
}
func verifyFile(path, expected string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expected), nil
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, path)
}
func within(root, path string) bool {
	r, _ := filepath.Abs(root)
	p, _ := filepath.Abs(path)
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
