package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBucketListCommandPrintsObjects(t *testing.T) {
	const clusterName = "bucket-cluster"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/buckets/sample" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"objects":[
				{"name":"foo.txt","size":12,"last_modified":"2024-01-01T10:00:00Z"},
				{"name":"bar.log","size":42,"last_modified":"2024-01-02T11:00:00Z"}
			]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"bucket", "--config", configFile,
		"list", "sample",
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "foo.txt") || !strings.Contains(stdout, "bar.log") {
		t.Fatalf("unexpected bucket list output: %q", stdout)
	}
}

func TestBucketListCommandJSONOutput(t *testing.T) {
	const clusterName = "bucket-cluster-json"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/buckets/sample" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"objects":[
				{"name":"logs/foo.txt","size":12,"last_modified":"2024-01-01T10:00:00Z"},
				{"name":"data/bar.log","size":42,"last_modified":"2024-01-02T11:00:00Z"}
			]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"bucket", "--config", configFile,
		"list", "sample",
		"--cluster", clusterName,
		"--output", "json",
		"--prefix", "logs/",
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var objects []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(stdout), &objects); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if len(objects) != 1 || objects[0].Name != "logs/foo.txt" {
		t.Fatalf("unexpected json output: %v", objects)
	}
}
