package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
)

const maximumExecutableSize = 256 << 20

func expectedArchive(tag, goos, goarch string) (string, string, error) {
	version, err := normalizeTag(tag)
	if err != nil {
		return "", "", err
	}
	version = strings.TrimPrefix(version, "v")
	executable := "backpack"
	extension := ".tar.gz"
	switch {
	case goos == "windows" && goarch == "amd64":
		executable, extension = "backpack.exe", ".zip"
	case goos == "linux" && goarch == "amd64":
	case goos == "darwin" && goarch == "arm64":
	default:
		return "", "", fmt.Errorf("self-update is unavailable for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("backpack_%s_%s_%s%s", version, goos, goarch, extension), executable, nil
}

func findAsset(release Release, name string) (Asset, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			if !strings.HasPrefix(asset.URL, "https://") {
				return Asset{}, fmt.Errorf("release asset %q does not use HTTPS", name)
			}
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s is missing asset %q", release.TagName, name)
}

func parseChecksum(data []byte, filename string) (string, error) {
	found := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != filename {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("checksum file repeats %q", filename)
		}
		candidate := strings.ToLower(fields[0])
		decoded, err := hex.DecodeString(candidate)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("checksum for %q is not SHA-256", filename)
		}
		found = candidate
	}
	if found == "" {
		return "", fmt.Errorf("checksum file does not contain %q", filename)
	}
	return found, nil
}

func verifyArchive(data []byte, expected string) error {
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if digest != strings.ToLower(expected) {
		return fmt.Errorf("release archive SHA-256 mismatch: expected %s, got %s", expected, digest)
	}
	return nil
}

func extractExecutable(archiveName, executableName string, data []byte) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractZIPExecutable(executableName, data)
	}
	if strings.HasSuffix(archiveName, ".tar.gz") {
		return extractTarExecutable(executableName, data)
	}
	return nil, fmt.Errorf("unsupported release archive %q", archiveName)
}

func extractZIPExecutable(executableName string, data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open release zip: %w", err)
	}
	var result []byte
	for _, file := range reader.File {
		if path.Clean(file.Name) != executableName || strings.Contains(file.Name, `\`) {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("release archive repeats executable %q", executableName)
		}
		if !file.Mode().IsRegular() || file.UncompressedSize64 > maximumExecutableSize {
			return nil, fmt.Errorf("release executable has an unsafe type or size")
		}
		opened, openErr := file.Open()
		if openErr != nil {
			return nil, openErr
		}
		result, err = readLimited(opened)
		_ = opened.Close()
		if err != nil {
			return nil, err
		}
	}
	if result == nil {
		return nil, fmt.Errorf("release archive does not contain %q", executableName)
	}
	return result, nil
}

func extractTarExecutable(executableName string, data []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open release gzip: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var result []byte
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read release tar: %w", nextErr)
		}
		if path.Clean(header.Name) != executableName || strings.Contains(header.Name, `\`) {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("release archive repeats executable %q", executableName)
		}
		if header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > maximumExecutableSize {
			return nil, fmt.Errorf("release executable has an unsafe type or size")
		}
		result, err = readLimited(reader)
		if err != nil {
			return nil, err
		}
	}
	if result == nil {
		return nil, fmt.Errorf("release archive does not contain %q", executableName)
	}
	return result, nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximumExecutableSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maximumExecutableSize {
		return nil, fmt.Errorf("release executable has an unsafe size")
	}
	return data, nil
}
