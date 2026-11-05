package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/grycap/oscar/v4/pkg/types"
	"gopkg.in/yaml.v3"
)

func TestHubDeployPreviewMatchesSelectedService(t *testing.T) {
	crate := t.TempDir()
	fdl := `functions:
  oscar:
  - zeta:
      name: second
      script: script.sh
      input:
      - storage_provider: minio.default
        path: second/input
    alpha:
      name: first
      script: script.sh
      input:
      - storage_provider: minio.default
        path: first/input
`
	if err := os.WriteFile(filepath.Join(crate, "fdl.yml"), []byte(fdl), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crate, "script.sh"), []byte("#!/bin/sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var deployed types.Service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/config":
			fmt.Fprint(w, `{"name":"oscar","namespace":"oscar","services_namespace":"oscar-svc"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/system/services/renamed":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/system/services":
			if err := json.NewDecoder(r.Body).Decode(&deployed); err != nil {
				t.Errorf("decoding deployment: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected cluster request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte(fmt.Sprintf("oscar:\n  test:\n    endpoint: %s\ndefault: test\n", server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	oldConfig := configPath
	configPath = configFile
	t.Cleanup(func() { configPath = oldConfig })

	for i := 0; i < 20; i++ {
		previewCmd := makeHubDeployCmd()
		var output bytes.Buffer
		previewCmd.SetOut(&output)
		previewCmd.SetArgs([]string{"demo", "--local-path", crate, "--name", "renamed", "--preview"})
		if err := previewCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var document struct {
			Functions struct {
				Oscar []map[string]struct {
					Name  string `yaml:"name"`
					Input []struct {
						Path string `yaml:"path"`
					} `yaml:"input"`
				} `yaml:"oscar"`
			} `yaml:"functions"`
		}
		if err := yaml.Unmarshal(output.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Functions.Oscar) != 1 || len(document.Functions.Oscar[0]) != 1 {
			t.Fatalf("preview must contain exactly one service: %s", output.String())
		}
		preview, ok := document.Functions.Oscar[0]["alpha"]
		if !ok || len(preview.Input) != 1 || preview.Input[0].Path != "renamed/input" {
			t.Fatalf("preview did not consistently select alpha: %s", output.String())
		}

		deployCmd := makeHubDeployCmd()
		deployCmd.SetOut(&bytes.Buffer{})
		deployCmd.SetArgs([]string{"demo", "--local-path", crate, "--name", "renamed", "--cluster", "test"})
		if err := deployCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if deployed.Name != preview.Name || len(deployed.Input) != 1 || deployed.Input[0].Path != preview.Input[0].Path {
			t.Fatalf("deployment differs from preview: name=%q input=%v; preview=%+v", deployed.Name, deployed.Input, preview)
		}
	}
}
