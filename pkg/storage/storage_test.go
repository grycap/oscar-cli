package storage

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

func TestDefaultRemotePath(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Input: []types.StorageIOConfig{
			{Provider: "minio.default", Path: "input"},
		},
	}

	got, err := DefaultRemotePath(svc, "minio.default", filepath.Join("tmp", "data.txt"))
	if err != nil {
		t.Fatalf("DefaultRemotePath returned error: %v", err)
	}
	if got != "input/data.txt" {
		t.Fatalf("expected input/data.txt, got %s", got)
	}
}

func TestDefaultRemotePathMissingInput(t *testing.T) {
	svc := &types.Service{
		Name:  "demo",
		Input: []types.StorageIOConfig{},
	}

	_, err := DefaultRemotePath(svc, "minio.default", "data.txt")
	if err == nil {
		t.Fatalf("expected error when input path missing")
	}
}

func TestDefaultOutputProvider(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Output: []types.StorageIOConfig{
			{Provider: "minio.results", Path: "output"},
		},
	}

	provider, err := DefaultOutputProvider(svc)
	if err != nil {
		t.Fatalf("DefaultOutputProvider returned error: %v", err)
	}
	if provider != "minio.results" {
		t.Fatalf("expected provider minio.results, got %s", provider)
	}
}

func TestDefaultOutputPath(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Output: []types.StorageIOConfig{
			{Provider: "minio.results", Path: "results"},
			{Provider: "s3.backup", Path: "backup"},
		},
	}

	path, err := DefaultOutputPath(svc, "minio.results")
	if err != nil {
		t.Fatalf("DefaultOutputPath returned error: %v", err)
	}
	if path != "results" {
		t.Fatalf("expected results path, got %s", path)
	}

	path, err = DefaultOutputPath(svc, "")
	if err != nil {
		t.Fatalf("DefaultOutputPath empty provider returned error: %v", err)
	}
	if path != "results" {
		t.Fatalf("expected fallback results path, got %s", path)
	}
}

func TestDefaultOutputPathMissing(t *testing.T) {
	svc := &types.Service{
		Name:   "demo",
		Output: []types.StorageIOConfig{},
	}

	_, err := DefaultOutputPath(svc, "")
	if err == nil {
		t.Fatalf("expected error when no output paths defined")
	}
}

func TestGetProviderDefaultMinIO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"minio_provider":{"access_key":"ak","secret_key":"sk","region":"us-east-1","endpoint":"https://minio","verify":true}}`))
	}))
	defer server.Close()

	c := &cluster.Cluster{
		Endpoint:  server.URL,
		AuthUser:  "user",
		SSLVerify: true,
	}

	prov, err := getProvider(c, "minio", nil)
	if err != nil {
		t.Fatalf("getProvider returned error: %v", err)
	}
	minioProv, ok := prov.(*types.MinIOProvider)
	if !ok {
		t.Fatalf("expected MinIO provider, got %T", prov)
	}
	if minioProv.Endpoint != "https://minio" {
		t.Fatalf("expected endpoint https://minio, got %s", minioProv.Endpoint)
	}
}

func TestGetProviderFromServiceDefinition(t *testing.T) {
	svcProviders := &types.StorageProviders{
		S3: map[string]*types.S3Provider{
			"results": {
				AccessKey: "ak",
				SecretKey: "sk",
				Region:    "us-east-1",
			},
		},
	}
	prov, err := getProvider(&cluster.Cluster{}, types.S3Name+".results", svcProviders)
	if err != nil {
		t.Fatalf("getProvider returned error: %v", err)
	}
	if _, ok := prov.(*types.S3Provider); !ok {
		t.Fatalf("expected S3 provider, got %T", prov)
	}
}
