package cluster

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestCheckStatusCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		res := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}
		if err := CheckStatusCode(res); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	cases := []struct {
		name     string
		code     int
		body     string
		expected error
	}{
		{"unauthorized", 401, "", errors.New("invalid credentials")},
		{"not found", 404, "", errors.New("not found")},
		{"service not ready", 502, "", errors.New("the service is not ready yet, please wait until it's ready or check if something failed")},
		{"other", 418, "boom", errors.New("boom")},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body))}
			err := CheckStatusCode(res)
			if err == nil || err.Error() != tc.expected.Error() {
				t.Fatalf("expected error %q, got %v", tc.expected, err)
			}
		})
	}
}

func TestGetClusterInfo(t *testing.T) {
	const (
		username = "user"
		password = "pass"
		version  = "1.2.3"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/info" {
			http.NotFound(w, r)
			return
		}
		gotUser, gotPass, ok := r.BasicAuth()
		if !ok || gotUser != username || gotPass != password {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := json.NewEncoder(w).Encode(&types.Info{Version: version}); err != nil {
			t.Fatalf("encoding info: %v", err)
		}
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:     server.URL,
		AuthUser:     username,
		AuthPassword: password,
		SSLVerify:    true,
	}

	info, err := c.GetClusterInfo()
	if err != nil {
		t.Fatalf("GetClusterInfo returned error: %v", err)
	}
	if info.Version != version {
		t.Fatalf("expected version %s, got %s", version, info.Version)
	}
}

func TestGetClusterInfoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:  server.URL,
		AuthUser:  "user",
		SSLVerify: true,
	}

	_, err := c.GetClusterInfo()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "boom\n" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestGetClusterConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/config" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(&types.Config{Name: "oscar", ServicesNamespace: "ns"}); err != nil {
			t.Fatalf("encoding config: %v", err)
		}
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint: server.URL,
	}

	cfg, err := c.GetClusterConfig()
	if err != nil {
		t.Fatalf("GetClusterConfig returned error: %v", err)
	}
	if cfg.Name != "oscar" {
		t.Fatalf("expected name oscar, got %s", cfg.Name)
	}
	if cfg.ServicesNamespace != "ns" {
		t.Fatalf("expected services namespace ns, got %s", cfg.ServicesNamespace)
	}
}
