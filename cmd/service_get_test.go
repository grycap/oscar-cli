package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestServiceGetCommandPrintsYAML(t *testing.T) {
	const (
		clusterName = "get-cluster"
		serviceName = "demo"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/services/"+serviceName {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(&types.Service{
				Name:   serviceName,
				Image:  "ghcr.io/demo/demo:latest",
				CPU:    "100m",
				Memory: "128Mi",
			}); err != nil {
				t.Fatalf("encoding service response: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"service", "--config", configFile,
		"get", serviceName,
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("service get command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, expected := range []string{serviceName, "ghcr.io/demo/demo:latest", "cpu: 100m", "memory: 128Mi"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected output to contain %q, got %q", expected, stdout)
		}
	}

	if !strings.Contains(stdout, "name: "+serviceName) {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

func TestServiceGetCommandReturnsError(t *testing.T) {
	const (
		clusterName = "get-error-cluster"
		serviceName = "missing"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/services/"+serviceName {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	_, _, err := runCommand(t,
		"service", "--config", configFile,
		"get", serviceName,
		"--cluster", clusterName,
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "not found" {
		t.Fatalf("expected not found error, got %v", err)
	}
}
