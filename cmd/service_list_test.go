package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestServiceListCommandPrintsServices(t *testing.T) {
	const clusterName = "list-cluster"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/services" {
			w.Header().Set("Content-Type", "application/json")
			list := []*types.Service{
				{Name: "svc-a", Image: "img:a", CPU: "100m", Memory: "128Mi"},
				{Name: "svc-b", Image: "img:b", CPU: "200m", Memory: "256Mi"},
			}
			if err := json.NewEncoder(w).Encode(list); err != nil {
				t.Fatalf("encoding services: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"service", "--config", configFile,
		"list",
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("service list command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "NAME") || !strings.Contains(stdout, "svc-a") || !strings.Contains(stdout, "svc-b") {
		t.Fatalf("unexpected list output: %q", stdout)
	}
}

func TestServiceListCommandNoServices(t *testing.T) {
	const clusterName = "list-empty-cluster"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/services" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, _, err := runCommand(t,
		"service", "--config", configFile,
		"list",
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("service list command returned error: %v", err)
	}
	if !strings.Contains(stdout, "There are no services in the cluster") {
		t.Fatalf("expected empty services message, got %q", stdout)
	}
}

func TestServiceListCommandUnauthorized(t *testing.T) {
	const clusterName = "list-unauthorized-cluster"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/services" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	_, _, err := runCommand(t,
		"service", "--config", configFile,
		"list",
		"--cluster", clusterName,
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "invalid credentials" {
		t.Fatalf("expected invalid credentials error, got %v", err)
	}
}
