package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grycap/oscar-cli/pkg/cluster"
)

func TestClusterStatusCommandPrintsStatus(t *testing.T) {
	expected := cluster.StatusInfo{
		Cluster: cluster.ClusterStatus{
			NodesCount: 1,
			Metrics: cluster.ClusterMetrics{
				CPU: cluster.CPUMetrics{
					TotalFreeCores:     2,
					MaxFreeOnNodeCores: 2,
				},
			},
		},
		Oscar: cluster.OscarStatus{
			DeploymentName: "oscar-manager",
			Deployment: cluster.OscarDeployment{
				CreationTimestamp: time.Unix(1700000000, 0).UTC(),
			},
		},
		MinIO: cluster.MinioStatus{
			BucketsCount: 4,
			TotalObjects: 10,
		},
	}

	const (
		username = "user"
		password = "pass"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/status" {
			http.NotFound(w, r)
			return
		}
		gotUser, gotPass, ok := r.BasicAuth()
		if !ok || gotUser != username || gotPass != password {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encoding status: %v", err)
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, "alpha", server.URL)

	stdout, stderr, err := runCommand(t,
		"cluster", "--config", configFile,
		"status",
		"--cluster", "alpha",
	)
	if err != nil {
		t.Fatalf("cluster status command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "nodes_count: 1") {
		t.Fatalf("expected nodes_count in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "deployment_name: oscar-manager") {
		t.Fatalf("expected deployment_name in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "buckets_count: 4") {
		t.Fatalf("expected buckets_count in output, got %q", stdout)
	}
}

func TestClusterStatusCommandError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, "alpha", server.URL)

	_, _, err := runCommand(t,
		"cluster", "--config", configFile,
		"status",
		"--cluster", "alpha",
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "boom\n" {
		t.Fatalf("expected boom error, got %v", err)
	}
}
