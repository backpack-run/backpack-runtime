package catalogverify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

const testRevision = "0123456789012345678901234567890123456789"

func TestVerifyDetectsDriftAndValidatesManifestContract(t *testing.T) {
	manifest := `schema_version: 1
model:
  id: one
packages:
  - id: gguf
    format: gguf
    precision: Q4_K_M
    filename: model.gguf
    sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    size_bytes: 42
    runtime:
      provider: llama.cpp
      tested_revision: test
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			fmt.Fprint(w, `[{"id":"backpack-run/one","sha":"`+testRevision+`","siblings":[{"rfilename":"backpack-model.yaml"}]},{"id":"backpack-run/new","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","siblings":[{"rfilename":"backpack-model.yaml"}]}]`)
		case "/backpack-run/one/resolve/" + testRevision + "/backpack-model.yaml":
			fmt.Fprint(w, manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	trusted := catalog.Catalog{SchemaVersion: 1, Models: []catalog.Model{{ID: "one", Aliases: []string{"first"}, Repository: "backpack-run/one", Revision: testRevision, RuntimeEngine: "llama.cpp", Status: "supported"}}}
	report, err := (Verifier{BaseURL: server.URL, Organization: "backpack-run"}).Verify(context.Background(), trusted)
	if err != nil {
		t.Fatal(err)
	}
	if report.Discovered != 2 || report.Represented != 1 || report.StatusCounts["supported"] != 1 {
		t.Fatalf("unexpected report %#v", report)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "missing-from-catalog" {
		t.Fatalf("unexpected issues %#v", report.Issues)
	}
}

func TestVerifyReportsUnavailableCatalogPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	trusted := catalog.Catalog{SchemaVersion: 1, Models: []catalog.Model{{ID: "missing", Aliases: []string{"missing-alias"}, Repository: "backpack-run/missing", Revision: testRevision, RuntimeEngine: "llama.cpp", Status: "experimental"}}}
	report, err := (Verifier{BaseURL: server.URL, Organization: "backpack-run"}).Verify(context.Background(), trusted)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "missing-from-huggingface" {
		t.Fatalf("unexpected issues %#v", report.Issues)
	}
}
