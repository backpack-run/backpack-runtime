package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"testing"
)

func makeZIP(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write(data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func makeTarGZ(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	for name, data := range entries {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write(data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestChecksumsAndArchiveExtraction(t *testing.T) {
	for _, test := range []struct {
		name, executable string
		archive          []byte
	}{{"backpack_1.0.0_windows_amd64.zip", "backpack.exe", makeZIP(t, map[string][]byte{"backpack.exe": []byte("windows"), "README.md": []byte("readme")})}, {"backpack_1.0.0_linux_amd64.tar.gz", "backpack", makeTarGZ(t, map[string][]byte{"backpack": []byte("linux"), "LICENSE": []byte("license")})}} {
		digest := fmt.Sprintf("%x", sha256.Sum256(test.archive))
		parsed, err := parseChecksum([]byte(digest+"  "+test.name+"\n"), test.name)
		if err != nil || parsed != digest {
			t.Fatalf("checksum=%q err=%v", parsed, err)
		}
		if err = verifyArchive(test.archive, parsed); err != nil {
			t.Fatal(err)
		}
		executable, err := extractExecutable(test.name, test.executable, test.archive)
		if err != nil || len(executable) == 0 {
			t.Fatalf("executable=%q err=%v", executable, err)
		}
	}
}

func TestArchiveAndChecksumFailures(t *testing.T) {
	archive := makeZIP(t, map[string][]byte{"nested/backpack.exe": []byte("bad")})
	if _, err := extractExecutable("release.zip", "backpack.exe", archive); err == nil {
		t.Fatal("nested executable accepted")
	}
	duplicate := makeZIP(t, map[string][]byte{"backpack.exe": []byte("one"), "./backpack.exe": []byte("two")})
	if _, err := extractExecutable("release.zip", "backpack.exe", duplicate); err == nil {
		t.Fatal("duplicate executable accepted")
	}
	if _, err := parseChecksum([]byte("bad  release.zip\n"), "release.zip"); err == nil {
		t.Fatal("invalid checksum accepted")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("x")))
	if _, err := parseChecksum([]byte(digest+"  release.zip\n"+digest+"  release.zip\n"), "release.zip"); err == nil {
		t.Fatal("duplicate checksum accepted")
	}
	if err := verifyArchive([]byte("archive"), fmt.Sprintf("%064d", 0)); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}

func TestExpectedPlatformAssets(t *testing.T) {
	name, executable, err := expectedArchive("v0.1.0-alpha.1", "windows", "amd64")
	if err != nil || name != "backpack_0.1.0-alpha.1_windows_amd64.zip" || executable != "backpack.exe" {
		t.Fatalf("name=%q executable=%q err=%v", name, executable, err)
	}
	if _, _, err = expectedArchive("v1.0.0", "linux", "arm64"); err == nil {
		t.Fatal("unsupported release platform accepted")
	}
}
