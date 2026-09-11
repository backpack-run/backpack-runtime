package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

type fakeSource struct {
	release Release
	data    map[string][]byte
	request ReleaseRequest
}

func (f *fakeSource) Resolve(_ context.Context, request ReleaseRequest) (Release, error) {
	f.request = request
	return f.release, nil
}
func (f *fakeSource) Fetch(_ context.Context, url string) ([]byte, error) {
	data, ok := f.data[url]
	if !ok {
		return nil, fmt.Errorf("missing fake URL %s", url)
	}
	return data, nil
}

func TestServiceCheckAndPrepare(t *testing.T) {
	archiveName := "backpack_0.2.0_windows_amd64.zip"
	archive := makeZIP(t, map[string][]byte{"backpack.exe": []byte("new-binary")})
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	source := &fakeSource{release: Release{TagName: "v0.2.0", Assets: []Asset{{Name: archiveName, URL: "https://download.test/archive"}, {Name: "checksums.txt", URL: "https://download.test/checksums"}}}, data: map[string][]byte{"https://download.test/archive": archive, "https://download.test/checksums": []byte(digest + "  " + archiveName + "\n")}}
	service := Service{Source: source, GOOS: "windows", GOARCH: "amd64"}
	plan, err := service.Check(context.Background(), "0.1.0", ReleaseRequest{})
	if err != nil || !plan.Available || source.request.IncludePrerelease {
		t.Fatalf("plan=%#v request=%#v err=%v", plan, source.request, err)
	}
	prepared, err := service.Prepare(context.Background(), plan)
	if err != nil || string(prepared.Executable) != "new-binary" || prepared.SHA256 != digest {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
}

func TestStableDoesNotAdoptPrereleaseAndExplicitDowngradeIsAllowed(t *testing.T) {
	source := &fakeSource{release: Release{TagName: "v2.0.0-alpha.1", Assets: []Asset{{Name: "backpack_2.0.0-alpha.1_windows_amd64.zip", URL: "https://download.test/archive"}, {Name: "checksums.txt", URL: "https://download.test/checksums"}}}}
	service := Service{Source: source, GOOS: "windows", GOARCH: "amd64"}
	// Channel exclusion is enforced by the release source; verify the service
	// does not opt in when the caller did not request it.
	_, _ = service.Check(context.Background(), "1.0.0", ReleaseRequest{})
	if source.request.IncludePrerelease {
		t.Fatal("stable check opted into prereleases")
	}
	source.release.TagName = "v0.9.0"
	source.release.Assets[0].Name = "backpack_0.9.0_windows_amd64.zip"
	plan, err := service.Check(context.Background(), "1.0.0", ReleaseRequest{Version: "v0.9.0"})
	if err != nil || !plan.Available || !plan.Explicit {
		t.Fatalf("explicit plan=%#v err=%v", plan, err)
	}
}

func TestPrepareRejectsChecksumMismatch(t *testing.T) {
	archiveName := "backpack_1.0.0_windows_amd64.zip"
	source := &fakeSource{release: Release{TagName: "v1.0.0"}, data: map[string][]byte{"checksums": []byte(fmt.Sprintf("%064d  %s\n", 0, archiveName)), "archive": []byte("corrupt")}}
	service := Service{Source: source}
	plan := Plan{Target: source.release, Archive: Asset{Name: archiveName, URL: "archive"}, Checksums: Asset{Name: "checksums.txt", URL: "checksums"}, executableName: "backpack.exe"}
	if _, err := service.Prepare(context.Background(), plan); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
