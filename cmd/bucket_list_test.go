package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestBucketListCommandPaginationFlags(t *testing.T) {
	const (
		clusterName = "bucket-cluster-page"
		pageToken   = "page-123"
	)

	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		fmt.Fprint(w, `{"objects":[{"object_name":"foo","size_bytes":1}],"next_page":"token-2","is_truncated":true}`)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, _, err := runCommand(t,
		"bucket", "--config", configFile,
		"list", "sample",
		"--cluster", clusterName,
		"--limit", "5",
		"--page", pageToken,
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if captured.Get("limit") != "5" || captured.Get("page") != pageToken {
		t.Fatalf("unexpected query parameters: %v", captured)
	}
	if !strings.Contains(stdout, "More objects are available") {
		t.Fatalf("expected pagination hint, got %q", stdout)
	}
}

func TestBucketListCommandAllPagesFlag(t *testing.T) {
	const clusterName = "bucket-cluster-all"

	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			fmt.Fprint(w, `{"objects":[{"object_name":"foo","size_bytes":1}],"next_page":"token-2","is_truncated":true,"returned_items":1}`)
			return
		}
		if r.URL.Query().Get("page") != "token-2" {
			t.Fatalf("expected token-2 page, got %q", r.URL.Query().Get("page"))
		}
		fmt.Fprint(w, `{"objects":[{"object_name":"bar","size_bytes":2}],"is_truncated":false,"returned_items":1}`)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, _, err := runCommand(t,
		"bucket", "--config", configFile,
		"list", "sample",
		"--cluster", clusterName,
		"--all",
	)
	if err != nil {
		t.Fatalf("bucket list returned error: %v", err)
	}
	if call != 2 {
		t.Fatalf("expected 2 server calls, got %d", call)
	}
	if strings.Contains(stdout, "More objects are available") {
		t.Fatalf("unexpected pagination hint when --all provided: %q", stdout)
	}
	if !strings.Contains(stdout, "foo") || !strings.Contains(stdout, "bar") {
		t.Fatalf("expected both objects in output, got %q", stdout)
	}
}
