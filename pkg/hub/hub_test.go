package hub_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/pkg/hub"
)

func TestClientListServices(t *testing.T) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/foo/bar/contents":
			writeJSON(t, w, []map[string]any{
				{"name": "svc1", "path": "svc1", "type": "dir"},
				{"name": "README.md", "path": "README.md", "type": "file"},
				{"name": "svc2", "path": "svc2", "type": "dir"},
			})
		case "/repos/foo/bar/contents/svc1/ro-crate-metadata.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(sampleROCrate("Example Service", "Alice Builder")))
		case "/repos/foo/bar/contents/svc2/ro-crate-metadata.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{ invalid json }"))
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := hub.NewClient(
		hub.WithOwner("foo"),
		hub.WithRepo("bar"),
		hub.WithHTTPClient(ts.Client()),
		hub.WithBaseAPI(ts.URL),
	)

	result, err := client.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices returned error: %v", err)
	}

	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(result.Services))
	}

	service := result.Services[0]
	if service.Slug != "svc1" {
		t.Errorf("expected slug svc1, got %s", service.Slug)
	}
	if service.Name != "Example Service" {
		t.Errorf("expected name Example Service, got %s", service.Name)
	}
	if service.Creator != "Alice Builder" {
		t.Errorf("expected creator Alice Builder, got %s", service.Creator)
	}
	expectedRepoURL := "https://github.com/foo/bar/tree/main/svc1"
	if service.RepositoryURL != expectedRepoURL {
		t.Errorf("expected repository URL %s, got %s", expectedRepoURL, service.RepositoryURL)
	}
	if service.MetadataSource != "svc1/ro-crate-metadata.json" {
		t.Errorf("expected metadata source svc1/ro-crate-metadata.json, got %s", service.MetadataSource)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Path != "svc2" {
		t.Errorf("expected warning path svc2, got %s", result.Warnings[0].Path)
	}
}

func TestClientListServicesWithRootPath(t *testing.T) {
	var requestedRoot bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/foo/bar/contents/services":
			requestedRoot = true
			writeJSON(t, w, []map[string]any{
				{"name": "svc", "path": "services/svc", "type": "dir"},
			})
		case "/repos/foo/bar/contents/services/svc/ro-crate-metadata.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(sampleROCrate("Nested Service", "Builder")))
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := hub.NewClient(
		hub.WithOwner("foo"),
		hub.WithRepo("bar"),
		hub.WithHTTPClient(ts.Client()),
		hub.WithBaseAPI(ts.URL),
		hub.WithRootPath("services"),
	)

	if _, err := client.ListServices(context.Background()); err != nil {
		t.Fatalf("ListServices returned error: %v", err)
	}

	if !requestedRoot {
		t.Fatalf("expected request to include services root path")
	}
}

func TestWithRootPathDot(t *testing.T) {
	var requestedPath string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		switch r.URL.Path {
		case "/repos/foo/bar/contents":
			writeJSON(t, w, []map[string]any{})
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := hub.NewClient(
		hub.WithOwner("foo"),
		hub.WithRepo("bar"),
		hub.WithHTTPClient(ts.Client()),
		hub.WithBaseAPI(ts.URL),
		hub.WithRootPath("."),
	)

	if _, err := client.ListServices(context.Background()); err != nil {
		t.Fatalf("ListServices returned error: %v", err)
	}

	if requestedPath != "/repos/foo/bar/contents" {
		t.Fatalf("expected request path /repos/foo/bar/contents, got %s", requestedPath)
	}
}

func TestFetchFDL(t *testing.T) {
	const (
		slug       = "demo"
		fdlContent = `
functions:
  oscar:
    - default:
        name: Demo Service
        image: example/demo:latest
        script: script.sh
`
		scriptContent = "#!/bin/bash\necho demo\n"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/foo/hub/contents/demo":
			writeJSON(t, w, []map[string]any{
				{"name": "demo.yaml", "path": "demo/demo.yaml", "type": "file"},
				{"name": "script.sh", "path": "demo/script.sh", "type": "file"},
			})
		case "/repos/foo/hub/contents/demo/demo.yaml":
			w.Write([]byte(fdlContent))
		case "/repos/foo/hub/contents/demo/script.sh":
			w.Write([]byte(scriptContent))
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := hub.NewClient(
		hub.WithOwner("foo"),
		hub.WithRepo("hub"),
		hub.WithBaseAPI(ts.URL),
		hub.WithHTTPClient(ts.Client()),
	)

	fdl, err := client.FetchFDL(context.Background(), slug)
	if err != nil {
		t.Fatalf("FetchFDL returned error: %v", err)
	}

	var serviceFound bool
	for _, element := range fdl.Functions.Oscar {
		for _, svc := range element {
			if svc == nil {
				continue
			}
			serviceFound = true
			if svc.Script != scriptContent {
				t.Fatalf("expected script content %q, got %q", scriptContent, svc.Script)
			}
		}
	}

	if !serviceFound {
		t.Fatalf("expected at least one service in FDL")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func sampleROCrate(name, creator string) string {
	lowerName := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return `{
  "@context": "https://w3id.org/ro/crate/1.1/context",
  "@graph": [
    {
      "@id": "ro-crate-metadata.json",
      "@type": "CreativeWork",
      "about": { "@id": "./" }
    },
    {
      "@id": "./",
      "@type": "Dataset",
      "name": "` + name + `",
      "description": "Test description for ` + name + `",
      "URL": "https://example.org/` + lowerName + `",
      "author": { "@id": "https://example.org/people/` + lowerName + `" },
      "license": { "@id": "https://spdx.org/licenses/MIT.html" }
    },
    {
      "@id": "https://example.org/people/` + lowerName + `",
      "@type": "Person",
      "name": "` + creator + `"
    },
    {
      "@id": "https://spdx.org/licenses/MIT.html",
      "@type": "CreativeWork",
      "name": "MIT License"
    }
  ]
}`
}
