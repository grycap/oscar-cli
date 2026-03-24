package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/pkg/storage"
)

func TestBucketListCommandPrintsObjects(t *testing.T) {
	const clusterName = "bucket-cluster"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/buckets" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{"bucket_name":"foo","visibility":"restricted","provider":"-","allowed_users":["user1","user2"],"owner":"owner1"},
				{"bucket_name":"bar","visibility":"restricted","provider":"-","allowed_users":["user3"],"owner":"owner2"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"bucket", "--config", configFile,
		"list",
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "foo") || !strings.Contains(stdout, "bar") {
		t.Fatalf("unexpected bucket list output: %q", stdout)
	}
}

func TestBucketListCommandJSONOutput(t *testing.T) {
	const clusterName = "bucket-cluster-json"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/system/buckets" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{"bucket_name":"foo","visibility":"restricted","provider":"-","allowed_users":["user1","user2"],"owner":"owner1"},
				{"bucket_name":"bar","visibility":"restricted","provider":"-","allowed_users":["user3"],"owner":"owner2"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"bucket", "--config", configFile,
		"list",
		"--cluster", clusterName,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var objects []storage.BucketInfo
	if err := json.Unmarshal([]byte(stdout), &objects); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if len(objects) != 2 || objects[0].Name != "bar" {
		t.Fatalf("unexpected json output: %v", objects)
	}
}

func TestBucketListCommandPublicBucket(t *testing.T) {
	const (
		clusterName = "bucket-cluster-page"
		pageToken   = "page-123"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
				{"bucket_name":"foo","visibility":"public","provider":"-","allowed_users":[],"owner":"owner1"}
			]`)
		return
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, _, err := runCommand(t,
		"bucket", "--config", configFile,
		"list",
		"--cluster", clusterName,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}

	var objects []storage.BucketInfo
	if err := json.Unmarshal([]byte(stdout), &objects); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if len(objects) != 1 || objects[0].Visibility != "public" {
		t.Fatalf("unexpected json output: %v", objects)
	}
}

func TestBucketListCommandPrivateBucket(t *testing.T) {
	const clusterName = "bucket-cluster-all"

	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			fmt.Fprint(w, `[
				{"bucket_name":"foo","visibility":"private","provider":"-","allowed_users":[],"owner":"owner1"}
			]`)
			return
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, _, err := runCommand(t,
		"bucket", "--config", configFile,
		"list",
		"--cluster", clusterName,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	var objects []storage.BucketInfo
	if err := json.Unmarshal([]byte(stdout), &objects); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if len(objects) != 1 || objects[0].Visibility != "private" {
		t.Fatalf("unexpected json output: %v", objects)
	}
}
