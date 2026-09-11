package updater

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func jsonClient(t *testing.T, expectedPath, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Scheme != "https" || request.URL.RequestURI() != expectedPath {
			t.Fatalf("unexpected request URL: %s", request.URL)
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestStableResolutionUsesLatestEndpoint(t *testing.T) {
	g := NewGitHub("backpack-run/backpack-runtime", jsonClient(t, "/repos/backpack-run/backpack-runtime/releases/latest", `{"tag_name":"v1.0.0","prerelease":false,"assets":[]}`))
	release, err := g.Resolve(context.Background(), ReleaseRequest{})
	if err != nil || release.TagName != "v1.0.0" {
		t.Fatalf("release=%#v err=%v", release, err)
	}
}

func TestPrereleaseResolutionSelectsHighestNonDraftVersion(t *testing.T) {
	body := `[
        {"tag_name":"v0.2.0-alpha.1","prerelease":true},
        {"tag_name":"v9.0.0","draft":true},
        {"tag_name":"not-semver"},
        {"tag_name":"v0.1.0","prerelease":false}
    ]`
	g := NewGitHub("backpack-run/backpack-runtime", jsonClient(t, "/repos/backpack-run/backpack-runtime/releases?per_page=100", body))
	release, err := g.Resolve(context.Background(), ReleaseRequest{IncludePrerelease: true})
	if err != nil || release.TagName != "v0.2.0-alpha.1" {
		t.Fatalf("release=%#v err=%v", release, err)
	}
}

func TestExactVersionResolution(t *testing.T) {
	g := NewGitHub("backpack-run/backpack-runtime", jsonClient(t, "/repos/backpack-run/backpack-runtime/releases/tags/v0.1.0-alpha.1", `{"tag_name":"v0.1.0-alpha.1","prerelease":true}`))
	if release, err := g.Resolve(context.Background(), ReleaseRequest{Version: "0.1.0-alpha.1"}); err != nil || release.TagName != "v0.1.0-alpha.1" {
		t.Fatalf("release=%#v err=%v", release, err)
	}
}

func TestExactVersionRejectsMismatchedResponse(t *testing.T) {
	g := NewGitHub("backpack-run/backpack-runtime", jsonClient(t, "/repos/backpack-run/backpack-runtime/releases/tags/v1.0.0", `{"tag_name":"v2.0.0"}`))
	if _, err := g.Resolve(context.Background(), ReleaseRequest{Version: "v1.0.0"}); err == nil {
		t.Fatal("mismatched exact release accepted")
	}
}

func TestFetchRejectsNonHTTPS(t *testing.T) {
	g := NewGitHub("backpack-run/backpack-runtime", http.DefaultClient)
	if _, err := g.Fetch(context.Background(), "http://example.test/file"); err == nil {
		t.Fatal("non-HTTPS release URL accepted")
	}
}
