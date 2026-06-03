package hub_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/v2/pkg/hub"
	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/grycap/oscar/v4/pkg/types"
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

func TestFetchFDLRendersAgentTemplate(t *testing.T) {
	const (
		slug          = "agents/pdf-summarizer"
		soulContent   = "# PDF Summarizer Soul\n\nSummarize PDFs carefully.\n"
		skillContent  = "# PDF Extract Skill\n\nUse pdftotext when available.\n"
		scriptContent = "#!/bin/sh\necho agent\n"
		fdlContent    = `
functions:
  oscar:
    - default:
        name: pdf-summarizer
        image: ghcr.io/demo/pdf:latest
        script: ../../frameworks/hermes/script.sh
        environment:
          variables:
            AGENT_SOUL: |
              {{ file "SOUL.md" }}
            AGENT_SKILLS: |
              {{ agentSkills }}
`
		metadataContent = `{
  "@context": ["https://w3id.org/ro/crate/1.1/context"],
  "@graph": [
    {
      "@id": "./",
      "@type": ["Dataset", "Service", "SoftwareApplication", "Agent"],
      "name": "PDF Summarizer Agent",
      "agentSkills": [
        { "@id": "../../skills/pdf-extract/SKILL.md" },
        { "@id": "https://www.skills.sh/openai/skills/pdf" }
      ]
    },
    {
      "@id": "../../skills/pdf-extract/SKILL.md",
      "@type": ["File", "CreativeWork", "AgentSkill"],
      "name": "PDF Extract Skill",
      "skillSource": "local",
      "encodingFormat": "text/markdown"
    },
    {
      "@id": "https://www.skills.sh/openai/skills/pdf",
      "@type": ["CreativeWork", "AgentSkill"],
      "name": "OpenAI PDF Skill",
      "description": "External PDF processing skill.",
      "url": "https://www.skills.sh/openai/skills/pdf",
      "skillSource": "marketplace"
    }
  ]
}`
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/foo/hub/contents/agents/pdf-summarizer":
			writeJSON(t, w, []map[string]any{
				{"name": "fdl.yml", "path": "agents/pdf-summarizer/fdl.yml", "type": "file"},
			})
		case "/repos/foo/hub/contents/agents/pdf-summarizer/fdl.yml":
			w.Write([]byte(fdlContent))
		case "/repos/foo/hub/contents/agents/pdf-summarizer/ro-crate-metadata.json":
			w.Write([]byte(metadataContent))
		case "/repos/foo/hub/contents/agents/pdf-summarizer/SOUL.md":
			w.Write([]byte(soulContent))
		case "/repos/foo/hub/contents/frameworks/hermes/script.sh":
			w.Write([]byte(scriptContent))
		case "/repos/foo/hub/contents/skills/pdf-extract/SKILL.md":
			w.Write([]byte(skillContent))
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

	svc := firstService(t, fdl)
	if svc.Script != scriptContent {
		t.Fatalf("expected shared script content %q, got %q", scriptContent, svc.Script)
	}
	if got := svc.Environment.Vars["AGENT_SOUL"]; !strings.Contains(got, "Summarize PDFs carefully.") {
		t.Fatalf("expected rendered soul content, got %q", got)
	}
	skills := svc.Environment.Vars["AGENT_SKILLS"]
	if !strings.Contains(skills, "Use pdftotext when available.") {
		t.Fatalf("expected local skill content, got %q", skills)
	}
	if !strings.Contains(skills, "https://www.skills.sh/openai/skills/pdf") {
		t.Fatalf("expected marketplace skill reference, got %q", skills)
	}
}

func TestLoadLocalFDLRendersAgentTemplate(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agents", "pdf-summarizer")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("creating agent dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "frameworks", "hermes"), 0o755); err != nil {
		t.Fatalf("creating framework dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "pdf-extract"), 0o755); err != nil {
		t.Fatalf("creating skills dir: %v", err)
	}

	writeFile(t, filepath.Join(agentDir, "fdl.yml"), `
functions:
  oscar:
    - default:
        name: pdf-summarizer
        image: ghcr.io/demo/pdf:latest
        script: ../../frameworks/hermes/script.sh
        environment:
          variables:
            AGENT_SOUL: |
              {{ file "SOUL.md" }}
            AGENT_SKILLS: |
              {{ agentSkills }}
`)
	writeFile(t, filepath.Join(agentDir, "SOUL.md"), "# Soul\n\nLocal soul.\n")
	writeFile(t, filepath.Join(agentDir, "ro-crate-metadata.json"), `{
  "@context": ["https://w3id.org/ro/crate/1.1/context"],
  "@graph": [
    {
      "@id": "./",
      "@type": ["Dataset", "Service", "SoftwareApplication", "Agent"],
      "agentSkills": [{ "@id": "../../skills/pdf-extract/SKILL.md" }]
    },
    {
      "@id": "../../skills/pdf-extract/SKILL.md",
      "@type": ["File", "CreativeWork", "AgentSkill"],
      "name": "PDF Extract Skill",
      "skillSource": "local",
      "encodingFormat": "text/markdown"
    }
  ]
}`)
	writeFile(t, filepath.Join(root, "frameworks", "hermes", "script.sh"), "#!/bin/sh\necho local\n")
	writeFile(t, filepath.Join(root, "skills", "pdf-extract", "SKILL.md"), "# Skill\n\nLocal skill.\n")

	fdl, err := hub.LoadLocalFDL(root, "agents/pdf-summarizer")
	if err != nil {
		t.Fatalf("LoadLocalFDL returned error: %v", err)
	}

	svc := firstService(t, fdl)
	if !strings.Contains(svc.Script, "echo local") {
		t.Fatalf("expected shared script content, got %q", svc.Script)
	}
	if got := svc.Environment.Vars["AGENT_SOUL"]; !strings.Contains(got, "Local soul.") {
		t.Fatalf("expected rendered soul content, got %q", got)
	}
	if got := svc.Environment.Vars["AGENT_SKILLS"]; !strings.Contains(got, "Local skill.") {
		t.Fatalf("expected rendered skill content, got %q", got)
	}
}

func TestLoadLocalFDLRendersAgentTemplateFromAgentsRoot(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agents", "pdf-summarizer")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("creating agent dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "frameworks", "hermes"), 0o755); err != nil {
		t.Fatalf("creating framework dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "pdf-extract"), 0o755); err != nil {
		t.Fatalf("creating skills dir: %v", err)
	}

	writeFile(t, filepath.Join(agentDir, "fdl.yml"), `
functions:
  oscar:
    - default:
        name: pdf-summarizer
        image: ghcr.io/demo/pdf:latest
        script: ../../frameworks/hermes/script.sh
        environment:
          variables:
            AGENT_SOUL: |
              {{ file "SOUL.md" }}
            AGENT_SKILLS: |
              {{ agentSkills }}
`)
	writeFile(t, filepath.Join(agentDir, "SOUL.md"), "# Soul\n\nAgents root soul.\n")
	writeFile(t, filepath.Join(agentDir, "ro-crate-metadata.json"), `{
  "@context": ["https://w3id.org/ro/crate/1.1/context"],
  "@graph": [
    {
      "@id": "./",
      "@type": ["Dataset", "Service", "SoftwareApplication", "Agent"],
      "agentSkills": [{ "@id": "../../skills/pdf-extract/SKILL.md" }]
    },
    {
      "@id": "../../skills/pdf-extract/SKILL.md",
      "@type": ["File", "CreativeWork", "AgentSkill"],
      "name": "PDF Extract Skill",
      "skillSource": "local",
      "encodingFormat": "text/markdown"
    }
  ]
}`)
	writeFile(t, filepath.Join(root, "frameworks", "hermes", "script.sh"), "#!/bin/sh\necho agents-root\n")
	writeFile(t, filepath.Join(root, "skills", "pdf-extract", "SKILL.md"), "# Skill\n\nAgents root skill.\n")

	fdl, err := hub.LoadLocalFDL(filepath.Join(root, "agents"), "pdf-summarizer")
	if err != nil {
		t.Fatalf("LoadLocalFDL returned error: %v", err)
	}

	svc := firstService(t, fdl)
	if !strings.Contains(svc.Script, "echo agents-root") {
		t.Fatalf("expected shared script content, got %q", svc.Script)
	}
	if got := svc.Environment.Vars["AGENT_SOUL"]; !strings.Contains(got, "Agents root soul.") {
		t.Fatalf("expected rendered soul content, got %q", got)
	}
	if got := svc.Environment.Vars["AGENT_SKILLS"]; !strings.Contains(got, "Agents root skill.") {
		t.Fatalf("expected rendered skill content, got %q", got)
	}
}

func firstService(t *testing.T, fdl *service.FDL) *types.Service {
	t.Helper()
	for _, element := range fdl.Functions.Oscar {
		for _, svc := range element {
			if svc != nil {
				return svc
			}
		}
	}
	t.Fatalf("expected at least one service in FDL")
	return nil
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
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
