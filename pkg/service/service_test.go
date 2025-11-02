package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

func TestReadFDL(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho hi\n"), 0o700); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	fdlPath := filepath.Join(dir, "service.yaml")
	content := `
functions:
  oscar:
    - default:
        name: demo
        image: ghcr.io/demo/app:latest
        script: script.sh
        cpu: 100m
        memory: 256Mi
`
	if err := os.WriteFile(fdlPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fdl: %v", err)
	}

	fdl, err := ReadFDL(fdlPath)
	if err != nil {
		t.Fatalf("ReadFDL returned error: %v", err)
	}
	if len(fdl.Functions.Oscar) != 1 {
		t.Fatalf("expected one function definition")
	}
	svc := fdl.Functions.Oscar[0]["default"]
	if svc == nil {
		t.Fatalf("expected service entry for cluster default")
	}
	if svc.ClusterID != "default" {
		t.Fatalf("expected ClusterID default, got %s", svc.ClusterID)
	}
	if !strings.Contains(svc.Script, "echo hi") {
		t.Fatalf("expected embedded script content, got %q", svc.Script)
	}
}

func TestReadFDLMissingScript(t *testing.T) {
	dir := t.TempDir()
	fdlPath := filepath.Join(dir, "service.yaml")
	content := `
functions:
  oscar:
    - default:
        name: demo
        image: ghcr.io/demo/app:latest
        script: missing.sh
`
	if err := os.WriteFile(fdlPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fdl: %v", err)
	}

	_, err := ReadFDL(fdlPath)
	if err == nil {
		t.Fatalf("expected error for missing script")
	}
	if !strings.Contains(err.Error(), "cannot load the script") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyService(t *testing.T) {
	const (
		serviceName = "demo"
		username    = "user"
		password    = "pass"
	)

	var received types.Service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/system/services":
			gotUser, gotPass, ok := r.BasicAuth()
			if !ok || gotUser != username || gotPass != password {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decoding payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := &cluster.Cluster{
		Endpoint:     server.URL,
		AuthUser:     username,
		AuthPassword: password,
		SSLVerify:    true,
	}

	err := ApplyService(&types.Service{Name: serviceName}, c, http.MethodPost)
	if err != nil {
		t.Fatalf("ApplyService returned error: %v", err)
	}
	if received.Name != serviceName {
		t.Fatalf("expected service name %s, got %s", serviceName, received.Name)
	}
}

func TestRunServiceUsesServiceToken(t *testing.T) {
	const (
		clusterName  = "cluster"
		serviceName  = "demo"
		serviceToken = "svc-token"
		responseBody = "RUN OK"
		payload      = "request"
	)

	var runAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/services/"+serviceName:
			if err := json.NewEncoder(w).Encode(&types.Service{Name: serviceName, Token: serviceToken}); err != nil {
				t.Fatalf("encoding service: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName:
			runAuth = r.Header.Get("Authorization")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading body: %v", err)
			}
			if strings.TrimSpace(string(body)) != payload {
				t.Fatalf("expected payload %q, got %q", payload, string(body))
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(responseBody)); err != nil {
				t.Fatalf("writing response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := &cluster.Cluster{
		Endpoint:  server.URL,
		AuthUser:  "user",
		SSLVerify: true,
	}

	resp, err := RunService(c, serviceName, "", "", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("RunService returned error: %v", err)
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	if string(body) != responseBody {
		t.Fatalf("expected response %q, got %q", responseBody, string(body))
	}
	if runAuth != "Bearer "+serviceToken {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer "+serviceToken, runAuth)
	}
}

func TestRunServiceWithProvidedToken(t *testing.T) {
	const (
		serviceName = "demo"
		token       = "provided"
		payload     = "hello"
	)

	var runAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName {
			runAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("OK")); err != nil {
				t.Fatalf("writing response: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resp, err := RunService(&cluster.Cluster{Endpoint: server.URL, SSLVerify: true}, serviceName, token, server.URL, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("RunService returned error: %v", err)
	}
	defer resp.Close()

	if runAuth != "Bearer "+token {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer "+token, runAuth)
	}
}
