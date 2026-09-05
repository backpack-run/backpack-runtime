package artifacts

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Artifact struct {
	ID              string    `json:"id"`
	MediaType       string    `json:"media_type"`
	Filename        string    `json:"filename"`
	SizeBytes       int64     `json:"size_bytes"`
	Format          string    `json:"format"`
	Width           int       `json:"width,omitempty"`
	Height          int       `json:"height,omitempty"`
	DurationSeconds float64   `json:"duration_seconds,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	Path            string    `json:"-"`
}

type Manager struct{ Root string }

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func (m Manager) JobDirectory(jobID string) (string, error) {
	if !safeID.MatchString(jobID) {
		return "", fmt.Errorf("invalid job ID")
	}
	directory := filepath.Join(m.Root, jobID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return directory, nil
}

func (m Manager) Inspect(jobID, artifactID, path string) (Artifact, error) {
	if !safeID.MatchString(artifactID) {
		return Artifact{}, fmt.Errorf("invalid artifact ID")
	}
	directory, err := m.JobDirectory(jobID)
	if err != nil {
		return Artifact{}, err
	}
	absDirectory, _ := filepath.Abs(directory)
	absPath, _ := filepath.Abs(path)
	relative, err := filepath.Rel(absDirectory, absPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Artifact{}, fmt.Errorf("artifact is outside the job output directory")
	}
	current := absDirectory
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, lstatErr := os.Lstat(current)
		if lstatErr != nil {
			return Artifact{}, lstatErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return Artifact{}, fmt.Errorf("artifact path must not contain symbolic links")
		}
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return Artifact{}, err
	}
	extension := strings.ToLower(filepath.Ext(absPath))
	result := Artifact{ID: artifactID, MediaType: mime.TypeByExtension(extension), Filename: filepath.Base(absPath), SizeBytes: info.Size(), Format: strings.TrimPrefix(extension, "."), CreatedAt: info.ModTime().UTC(), Path: absPath}
	if strings.HasPrefix(result.MediaType, "image/") {
		file, openErr := os.Open(absPath)
		if openErr == nil {
			if config, _, decodeErr := image.DecodeConfig(file); decodeErr == nil {
				result.Width, result.Height = config.Width, config.Height
			}
			_ = file.Close()
		}
	}
	if result.MediaType == "" {
		result.MediaType = "application/octet-stream"
	}
	return result, nil
}

func (m Manager) Resolve(jobID, artifactID string, items []Artifact) (string, error) {
	for _, item := range items {
		if item.ID == artifactID {
			verified, err := m.Inspect(jobID, artifactID, item.Path)
			return verified.Path, err
		}
	}
	return "", fmt.Errorf("artifact %q was not found", artifactID)
}
