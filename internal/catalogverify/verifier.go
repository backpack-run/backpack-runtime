package catalogverify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

const maxMetadataBytes = 4 << 20

type Client interface {
	Do(*http.Request) (*http.Response, error)
}

type Verifier struct {
	Client       Client
	BaseURL      string
	Organization string
}

type Issue struct {
	Repository string `json:"repository,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

type Report struct {
	Organization string         `json:"organization"`
	Discovered   int            `json:"discovered"`
	Represented  int            `json:"represented"`
	StatusCounts map[string]int `json:"status_counts"`
	Issues       []Issue        `json:"issues"`
}

type hfModel struct {
	ID       string `json:"id"`
	SHA      string `json:"sha"`
	Private  bool   `json:"private"`
	Siblings []struct {
		Filename string `json:"rfilename"`
	} `json:"siblings"`
}

func (v Verifier) Verify(ctx context.Context, trusted catalog.Catalog) (Report, error) {
	if err := trusted.Validate(); err != nil {
		return Report{}, err
	}
	organization := strings.TrimSpace(v.Organization)
	if organization == "" {
		organization = "backpack-run"
	}
	baseURL := strings.TrimRight(v.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://huggingface.co"
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	report := Report{Organization: organization, StatusCounts: map[string]int{}}
	for _, entry := range trusted.Models {
		report.StatusCounts[entry.Status]++
	}

	var discovered []hfModel
	endpoint := fmt.Sprintf("%s/api/models?author=%s&limit=100&full=true", baseURL, url.QueryEscape(organization))
	if err := getJSON(ctx, client, endpoint, &discovered); err != nil {
		return report, fmt.Errorf("discover Hugging Face organization: %w", err)
	}
	remote := map[string]hfModel{}
	for _, model := range discovered {
		if model.Private || !strings.EqualFold(strings.SplitN(model.ID, "/", 2)[0], organization) {
			continue
		}
		remote[strings.ToLower(model.ID)] = model
	}
	report.Discovered = len(remote)
	entries := map[string]catalog.Model{}
	for _, entry := range trusted.Models {
		entries[strings.ToLower(entry.Repository)] = entry
	}
	for key, model := range remote {
		if _, exists := entries[key]; !exists {
			report.Issues = append(report.Issues, Issue{Repository: model.ID, Code: "missing-from-catalog", Message: "public Backpack package is absent from the trusted catalog"})
		}
	}
	for _, entry := range trusted.Models {
		model, exists := remote[strings.ToLower(entry.Repository)]
		if !exists {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "missing-from-huggingface", Message: "trusted catalog entry has no public Backpack package"})
			continue
		}
		report.Represented++
		if !strings.EqualFold(model.SHA, entry.Revision) {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "revision-drift", Message: fmt.Sprintf("catalog pins %s but public package head is %s", entry.Revision, model.SHA)})
		}
		if !hasSibling(model, "backpack-model.yaml") {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "manifest-missing", Message: "public package lacks backpack-model.yaml"})
			continue
		}
		manifestURL := fmt.Sprintf("%s/%s/resolve/%s/backpack-model.yaml?download=true", baseURL, escapeRepository(entry.Repository), url.PathEscape(entry.Revision))
		data, err := getBytes(ctx, client, manifestURL)
		if err != nil {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "catalog-revision-unavailable", Message: err.Error()})
			continue
		}
		manifest, err := models.ParseManifest(data)
		if err != nil {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "manifest-incompatible", Message: err.Error()})
			continue
		}
		if !acceptedManifestID(entry, manifest.Model.ID) {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "manifest-id-mismatch", Message: fmt.Sprintf("catalog ID %q does not accept manifest ID %q", entry.ID, manifest.Model.ID)})
		}
		if err := verifyRuntimeContract(entry, manifest); err != nil {
			report.Issues = append(report.Issues, Issue{Repository: entry.Repository, Code: "runtime-contract-mismatch", Message: err.Error()})
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		return report.Issues[i].Repository+report.Issues[i].Code < report.Issues[j].Repository+report.Issues[j].Code
	})
	return report, nil
}

func getJSON(ctx context.Context, client Client, endpoint string, output any) error {
	data, err := getBytes(ctx, client, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func getBytes(ctx context.Context, client Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("GET %s returned %s", endpoint, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("metadata response exceeds %d bytes", maxMetadataBytes)
	}
	return data, nil
}

func escapeRepository(repository string) string {
	parts := strings.Split(repository, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func hasSibling(model hfModel, name string) bool {
	for _, sibling := range model.Siblings {
		if sibling.Filename == name {
			return true
		}
	}
	return false
}

func acceptedManifestID(entry catalog.Model, manifestID string) bool {
	if strings.EqualFold(entry.ID, manifestID) {
		return true
	}
	for _, accepted := range entry.ManifestIDs {
		if strings.EqualFold(accepted, manifestID) {
			return true
		}
	}
	return false
}

func verifyRuntimeContract(entry catalog.Model, manifest *models.Manifest) error {
	if manifest.Pipeline != nil {
		if !strings.EqualFold(manifest.Pipeline.Engine, entry.RuntimeEngine) {
			return fmt.Errorf("catalog engine %q differs from pipeline engine %q", entry.RuntimeEngine, manifest.Pipeline.Engine)
		}
		return nil
	}
	for _, pkg := range manifest.Packages {
		requirement := manifest.RuntimeFor(pkg)
		if requirement.Engine == "" {
			return fmt.Errorf("package %q has no normalized runtime engine", pkg.ID)
		}
		if !strings.EqualFold(requirement.Engine, entry.RuntimeEngine) {
			return fmt.Errorf("catalog engine %q differs from package %q engine %q", entry.RuntimeEngine, pkg.ID, requirement.Engine)
		}
	}
	return nil
}
