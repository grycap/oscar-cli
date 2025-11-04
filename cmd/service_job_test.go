package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestServiceJobCommandTextInput(t *testing.T) {
	const (
		clusterName  = "job-cluster"
		serviceName  = "batch"
		serviceToken = "job-token"
		payload      = "dataset"
	)

	var (
		receivedBody string
		authHeader   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/services/"+serviceName:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(&types.Service{
				Name:  serviceName,
				Token: serviceToken,
			}); err != nil {
				t.Fatalf("encoding service response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/job/"+serviceName:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading job payload: %v", err)
			}
			receivedBody = strings.TrimSpace(string(body))
			authHeader = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"service", "--config", configFile,
		"job", serviceName,
		"--cluster", clusterName,
		"--text-input", payload,
	)
	if err != nil {
		t.Fatalf("service job command returned error: %v", err)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	decoded, err := base64.StdEncoding.DecodeString(receivedBody)
	if err != nil {
		t.Fatalf("decoding job payload: %v", err)
	}
	if string(decoded) != payload {
		t.Fatalf("expected payload %q, got %q", payload, decoded)
	}
	if authHeader != "Bearer "+serviceToken {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer "+serviceToken, authHeader)
	}
}

func TestServiceJobCommandValidation(t *testing.T) {
	const clusterName = "job-validate-cluster"

	configFile := writeConfigFile(t, clusterName, "http://127.0.0.1:0")

	testCases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "missing input",
			args: []string{
				"service", "--config", configFile,
				"job", "svc",
				"--cluster", clusterName,
			},
			wantErr: `you must specify "--file-input" or "--text-input" flag`,
		},
		{
			name: "conflicting inputs",
			args: []string{
				"service", "--config", configFile,
				"job", "svc",
				"--cluster", clusterName,
				"--text-input", "data",
				"--file-input", "input.txt",
			},
			wantErr: `you only can specify one of "--file-input" or "--text-input" flags`,
		},
		{
			name: "token without endpoint",
			args: []string{
				"service", "--config", configFile,
				"job", "svc",
				"--cluster", clusterName,
				"--token", "abc",
				"--text-input", "data",
			},
			wantErr: `you must specify a the cluster endpoint with the flag "--endpoint"`,
		},
		{
			name: "endpoint without token",
			args: []string{
				"service", "--config", configFile,
				"job", "svc",
				"--cluster", clusterName,
				"--endpoint", "https://example.com",
				"--text-input", "data",
			},
			wantErr: `you must specify a service token with the flag "--token"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCommand(t, tc.args...)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
