package artifacts

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactInspectionAndConfinement(t *testing.T) {
	m := Manager{Root: t.TempDir()}
	directory, err := m.JobDirectory("job-one")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "result.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.White)
	if err = png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	artifact, err := m.Inspect("job-one", "output", path)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Width != 3 || artifact.Height != 2 || artifact.MediaType != "image/png" || artifact.Path == "" {
		t.Fatalf("artifact %#v", artifact)
	}
	outside := filepath.Join(m.Root, "outside.png")
	if err = os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Inspect("job-one", "escape", outside); err == nil {
		t.Fatal("outside artifact accepted")
	}
}

func TestArtifactRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	m := Manager{Root: filepath.Join(root, "outputs")}
	directory, err := m.JobDirectory("job-one")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "private.txt")
	if err = os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "result.txt")
	if err = os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err = m.Inspect("job-one", "output", link); err == nil {
		t.Fatal("symbolic-link escape accepted")
	}
}
