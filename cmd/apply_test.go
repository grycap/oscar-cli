package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar/v4/pkg/types"
)

func TestOverrideServiceNameUpdatesPaths(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Input: []types.StorageIOConfig{
			{Path: "demo/in"},
			{Path: "other/in"},
		},
		Output: []types.StorageIOConfig{{Path: "demo"}},
		Mount:  types.StorageIOConfig{Path: "demo/mount"},
	}

	overrideServiceName(svc, "demo-new")

	if svc.Name != "demo-new" {
		t.Fatalf("expected service name demo-new, got %s", svc.Name)
	}
	if got := svc.Input[0].Path; got != "demo-new/in" {
		t.Fatalf("expected first input path demo-new/in, got %s", got)
	}
	if got := svc.Input[1].Path; got != "other/in" {
		t.Fatalf("unexpected path rewrite: %s", got)
	}
	if got := svc.Output[0].Path; got != "demo-new" {
		t.Fatalf("expected output path demo-new, got %s", got)
	}
	if got := svc.Mount.Path; got != "demo-new/mount" {
		t.Fatalf("expected mount path demo-new/mount, got %s", got)
	}
}

func TestOverrideServiceNameUpdatesPathsWhenBucketDiffersFromServiceName(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Input: []types.StorageIOConfig{
			{Path: "workflow/in"},
			{Path: "other/in"},
		},
		Output: []types.StorageIOConfig{{Path: "workflow"}},
		Mount:  types.StorageIOConfig{Path: "workflow/mount"},
	}

	overrideServiceName(svc, "demo-new")

	if svc.Name != "demo-new" {
		t.Fatalf("expected service name demo-new, got %s", svc.Name)
	}
	if got := svc.Input[0].Path; got != "demo-new/in" {
		t.Fatalf("expected first input path demo-new/in, got %s", got)
	}
	if got := svc.Input[1].Path; got != "other/in" {
		t.Fatalf("unexpected path rewrite: %s", got)
	}
	if got := svc.Output[0].Path; got != "demo-new" {
		t.Fatalf("expected output path demo-new, got %s", got)
	}
	if got := svc.Mount.Path; got != "demo-new/mount" {
		t.Fatalf("expected mount path demo-new/mount, got %s", got)
	}
}

func TestReplacePathBucket(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		old      string
		new      string
		expected string
	}{
		{"simple", "demo", "demo", "new", "new"},
		{"withDir", "demo/files", "demo", "new", "new/files"},
		{"unmatched", "other", "demo", "new", "other"},
		{"leadingSlash", "/demo/files", "demo", "new", "/new/files"},
		{"trailingSlash", "demo/", "demo", "new", "new/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replacePathBucket(tc.path, tc.old, tc.new)
			if got != tc.expected {
				t.Fatalf("replacePathBucket(%q) = %q, want %q", tc.path, got, tc.expected)
			}
		})
	}
}

func TestApplyCommandUsesEnvFile(t *testing.T) {
	var applied types.Service
	clusterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/config":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"oscar","namespace":"oscar","services_namespace":"oscar-svc"}`)
		case r.Method == http.MethodGet && strings.EqualFold(r.URL.Path, "/system/services/demo"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/system/services":
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&applied); err != nil {
				t.Fatalf("decoding service apply payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected cluster request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer clusterServer.Close()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	fdlFile := filepath.Join(tmpDir, "fdl.yml")
	scriptFile := filepath.Join(tmpDir, "script.sh")
	envFile := filepath.Join(tmpDir, ".env")

	configContent := fmt.Sprintf(`oscar:
  test:
    endpoint: "%s"
    auth_user: ""
    auth_password: ""
    ssl_verify: false
    memory: 256Mi
    log_level: INFO
default: test
`, clusterServer.URL)
	if err := os.WriteFile(configFile, []byte(configContent), 0o600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
	if err := os.WriteFile(scriptFile, []byte("#!/bin/bash\necho ok\n"), 0o600); err != nil {
		t.Fatalf("writing script file: %v", err)
	}

	fdlContent := `
functions:
  oscar:
  - test:
      name: demo
      image: ghcr.io/demo/demo:latest
      script: script.sh
      environment:
        variables:
          OPENAI_BASE_URL: old-url
          OPENAI_MODEL: old-model
        secrets:
          OPENAI_API_KEY: old-secret
`
	if err := os.WriteFile(fdlFile, []byte(fdlContent), 0o600); err != nil {
		t.Fatalf("writing fdl file: %v", err)
	}

	envContent := `OPENAI_API_KEY=new-secret
OPENAI_BASE_URL=https://example.com/v1
OPENAI_MODEL=new-model
UNDECLARED=ignored
`
	if err := os.WriteFile(envFile, []byte(envContent), 0o600); err != nil {
		t.Fatalf("writing env file: %v", err)
	}

	cmd := makeApplyCmd()
	cmd.SetArgs([]string{fdlFile, "--config", configFile, "--cluster", "test", "--env-file", envFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply command returned error: %v", err)
	}

	if got := applied.Environment.Secrets["OPENAI_API_KEY"]; got != "new-secret" {
		t.Fatalf("expected OPENAI_API_KEY override, got %q", got)
	}
	if got := applied.Environment.Vars["OPENAI_BASE_URL"]; got != "https://example.com/v1" {
		t.Fatalf("expected OPENAI_BASE_URL override, got %q", got)
	}
	if got := applied.Environment.Vars["OPENAI_MODEL"]; got != "new-model" {
		t.Fatalf("expected OPENAI_MODEL override, got %q", got)
	}
	if _, ok := applied.Environment.Vars["UNDECLARED"]; ok {
		t.Fatal("expected undeclared env key to be ignored")
	}
	if _, ok := applied.Environment.Secrets["UNDECLARED"]; ok {
		t.Fatal("expected undeclared secret key to be ignored")
	}
}
