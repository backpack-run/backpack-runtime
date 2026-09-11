package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const defaultAPI = "https://api.github.com"

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type ReleaseRequest struct {
	Version           string
	IncludePrerelease bool
}

type ReleaseSource interface {
	Resolve(context.Context, ReleaseRequest) (Release, error)
	Fetch(context.Context, string) ([]byte, error)
}

type GitHub struct {
	Repository string
	APIBase    string
	HTTP       *http.Client
}

func NewGitHub(repository string, client *http.Client) *GitHub {
	if client == nil {
		client = secureHTTPClient()
	}
	return &GitHub{Repository: repository, APIBase: defaultAPI, HTTP: client}
}

func secureHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return fmt.Errorf("refusing non-HTTPS redirect to %s", request.URL)
		}
		if len(via) >= 8 {
			return fmt.Errorf("too many download redirects")
		}
		return nil
	}}
}

func (g *GitHub) Resolve(ctx context.Context, request ReleaseRequest) (Release, error) {
	if g == nil || !repositoryName.MatchString(g.Repository) || strings.Contains(g.Repository, "..") {
		return Release{}, fmt.Errorf("GitHub repository is required")
	}
	base := strings.TrimRight(g.APIBase, "/")
	if base == "" {
		base = defaultAPI
	}
	var endpoint string
	if request.Version != "" {
		tag, err := normalizeTag(request.Version)
		if err != nil {
			return Release{}, err
		}
		endpoint = base + "/repos/" + g.Repository + "/releases/tags/" + url.PathEscape(tag)
	} else if request.IncludePrerelease {
		endpoint = base + "/repos/" + g.Repository + "/releases?per_page=100"
	} else {
		// GitHub's latest-release endpoint excludes drafts and prereleases. This
		// prevents stable-channel users from moving to an alpha or beta silently.
		endpoint = base + "/repos/" + g.Repository + "/releases/latest"
	}
	if request.IncludePrerelease && request.Version == "" {
		var releases []Release
		if err := g.getJSON(ctx, endpoint, &releases); err != nil {
			return Release{}, err
		}
		return newestRelease(releases)
	}
	var release Release
	if err := g.getJSON(ctx, endpoint, &release); err != nil {
		return Release{}, err
	}
	if request.Version != "" {
		requestedTag, _ := normalizeTag(request.Version)
		actualTag, tagErr := normalizeTag(release.TagName)
		if tagErr != nil || requestedTag != actualTag {
			return Release{}, fmt.Errorf("GitHub returned release %q for requested tag %q", release.TagName, requestedTag)
		}
	}
	if release.Draft || (!request.IncludePrerelease && request.Version == "" && release.Prerelease) {
		return Release{}, fmt.Errorf("GitHub returned a release outside the requested channel")
	}
	if _, err := ParseVersion(release.TagName); err != nil {
		return Release{}, fmt.Errorf("release has invalid tag: %w", err)
	}
	return release, nil
}

func newestRelease(releases []Release) (Release, error) {
	var selected Release
	var selectedVersion Version
	found := false
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version, err := ParseVersion(release.TagName)
		if err != nil {
			continue
		}
		if !found || version.Compare(selectedVersion) > 0 {
			selected, selectedVersion, found = release, version, true
		}
	}
	if !found {
		return Release{}, fmt.Errorf("no valid GitHub releases were found")
	}
	return selected, nil
}

func (g *GitHub) getJSON(ctx context.Context, endpoint string, target any) error {
	body, err := g.Fetch(ctx, endpoint)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode GitHub release response: %w", err)
	}
	return nil
}

func (g *GitHub) Fetch(ctx context.Context, endpoint string) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("refusing non-HTTPS release URL %q", endpoint)
	}
	client := g.HTTP
	if client == nil {
		client = secureHTTPClient()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "backpack-runtime-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download release data: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", response.Status)
	}
	const maximumDownload = 512 << 20
	limited := io.LimitReader(response.Body, maximumDownload+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maximumDownload {
		return nil, fmt.Errorf("release download exceeds %d bytes", maximumDownload)
	}
	return body, nil
}
