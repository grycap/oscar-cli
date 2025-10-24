package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestHubDeployCommandCreatesService(t *testing.T) {
	const (
		slug       = "cowsay"
		override   = "alt-cowsay"
		fdlContent = `
functions:
  oscar:
    - default:
        name: Cowsay
        image: ghcr.io/demo/cowsay:latest
        script: script.sh
`
		scriptContent = "#!/bin/bash\necho moo\n"
	)

	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/grycap/oscar-hub/contents/cowsay", "/repos/grycap/oscar-hub/contents/crates/cowsay":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"name":"cowsay.yaml","path":"cowsay/cowsay.yaml","type":"file"},{"name":"script.sh","path":"cowsay/script.sh","type":"file"}]`)
		case "/repos/grycap/oscar-hub/contents/cowsay/cowsay.yaml", "/repos/grycap/oscar-hub/contents/crates/cowsay/cowsay.yaml":
			fmt.Fprint(w, fdlContent)
		case "/repos/grycap/oscar-hub/contents/cowsay/script.sh", "/repos/grycap/oscar-hub/contents/crates/cowsay/script.sh":
			fmt.Fprint(w, scriptContent)
		default:
			t.Fatalf("unexpected github request: %s", r.URL.Path)
		}
	}))
	defer gitServer.Close()

	var applied types.Service
	clusterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/config":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"oscar","namespace":"oscar","services_namespace":"oscar-svc"}`)
		case r.Method == http.MethodGet && strings.EqualFold(r.URL.Path, "/system/services/"+override):
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

	originalConfigPath := configPath
	configPath = configFile
	t.Cleanup(func() { configPath = originalConfigPath })

	cmd := makeHubDeployCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{slug, "--api-base", gitServer.URL, "--cluster", "test", "--name", override})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hub deploy command returned error: %v", err)
	}

	if applied.Name != override {
		t.Fatalf("expected service name %s, got %s", override, applied.Name)
	}
	if applied.Script != scriptContent {
		t.Fatalf("expected script content %q, got %q", scriptContent, applied.Script)
	}
	if applied.ClusterID != "test" {
		t.Fatalf("expected cluster id test, got %s", applied.ClusterID)
	}
	clusterInfo, ok := applied.Clusters["test"]
	if !ok {
		t.Fatalf("expected clusters map to include target cluster")
	}
	if clusterInfo.Endpoint != clusterServer.URL {
		t.Fatalf("expected cluster endpoint %s, got %s", clusterServer.URL, clusterInfo.Endpoint)
	}
}
