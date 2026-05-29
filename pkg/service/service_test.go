package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/v2/pkg/cluster"
	"github.com/grycap/oscar/v4/pkg/types"
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
        volume:
          size: 10Gi
          mount_path: /data
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
	if svc.Volume == nil {
		t.Fatalf("expected volume config to be preserved")
	}
	if svc.Volume.Size != "10Gi" {
		t.Fatalf("expected volume size 10Gi, got %s", svc.Volume.Size)
	}
	if svc.Volume.MountPath != "/data" {
		t.Fatalf("expected volume mount path /data, got %s", svc.Volume.MountPath)
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

	err := ApplyService(&types.Service{
		Name: serviceName,
		Volume: &types.ServiceVolumeConfig{
			Size:      "10Gi",
			MountPath: "/data",
		},
	}, c, http.MethodPost)
	if err != nil {
		t.Fatalf("ApplyService returned error: %v", err)
	}
	if received.Name != serviceName {
		t.Fatalf("expected service name %s, got %s", serviceName, received.Name)
	}
	if received.Volume == nil {
		t.Fatalf("expected volume config in applied service")
	}
	if received.Volume.Size != "10Gi" {
		t.Fatalf("expected applied volume size 10Gi, got %s", received.Volume.Size)
	}
	if received.Volume.MountPath != "/data" {
		t.Fatalf("expected applied volume mount path /data, got %s", received.Volume.MountPath)
	}
}

func TestNormalizeExposePortListsOmitsZeroNodePort(t *testing.T) {
	payload := []byte(`{"name":"demo","expose":{"min_scale":0,"max_scale":0,"nodePort":0}}`)

	normalized, err := normalizeExposePortLists(payload)
	if err != nil {
		t.Fatalf("normalizeExposePortLists returned error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(normalized, &got); err != nil {
		t.Fatalf("decoding normalized payload: %v", err)
	}
	expose := got["expose"].(map[string]interface{})
	if _, ok := expose["nodePort"]; ok {
		t.Fatalf("expected zero nodePort to be omitted, got %v", expose["nodePort"])
	}
}

func TestNormalizeExposePortListsConvertsScalarPorts(t *testing.T) {
	payload := []byte(`{"name":"demo","expose":{"api_port":8080,"nodePort":30080}}`)

	normalized, err := normalizeExposePortLists(payload)
	if err != nil {
		t.Fatalf("normalizeExposePortLists returned error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(normalized, &got); err != nil {
		t.Fatalf("decoding normalized payload: %v", err)
	}
	expose := got["expose"].(map[string]interface{})

	apiPort, ok := expose["api_port"].([]interface{})
	if !ok || len(apiPort) != 1 || apiPort[0].(float64) != 8080 {
		t.Fatalf("expected api_port [8080], got %#v", expose["api_port"])
	}

	nodePort, ok := expose["nodePort"].([]interface{})
	if !ok || len(nodePort) != 1 || nodePort[0].(float64) != 30080 {
		t.Fatalf("expected nodePort [30080], got %#v", expose["nodePort"])
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/services/"+serviceName:
			if err := json.NewEncoder(w).Encode(&types.Service{Name: serviceName, Token: serviceToken}); err != nil {
				t.Fatalf("encoding service: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName:
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

}

func TestRunServiceWithProvidedToken(t *testing.T) {
	const (
		serviceName = "demo"
		token       = "provided"
		payload     = "hello"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName {
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

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp)
	respParse := buf.String()
	fmt.Println(respParse)

	if respParse != "OK" {
		t.Fatalf("expected response OK, got %q", respParse)
	}
}
