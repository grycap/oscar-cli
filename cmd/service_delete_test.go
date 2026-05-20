package cmd

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestServiceDeleteCommandDeletesServices(t *testing.T) {
	const (
		clusterName = "delete-cluster"
		svcA        = "alpha"
		svcB        = "beta"
	)

	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/system/services/") {
			deleted = append(deleted, path.Base(r.URL.Path))
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	_, _, err := runCommand(t,
		"service", "--config", configFile,
		"delete", svcA, svcB,
		"--cluster", clusterName,
	)
	if err != nil {
		t.Fatalf("service delete command returned error: %v", err)
	}

	if len(deleted) != 2 || deleted[0] != svcA || deleted[1] != svcB {
		t.Fatalf("expected delete calls for %q and %q, got %v", svcA, svcB, deleted)
	}
}

func TestServiceDeleteCommandPropagatesErrors(t *testing.T) {
	const (
		clusterName = "delete-error-cluster"
		svcName     = "missing"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/system/services/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	_, _, err := runCommand(t,
		"service", "--config", configFile,
		"delete", svcName,
		"--cluster", clusterName,
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "not found" {
		t.Fatalf("expected not found error, got %v", err)
	}
}
