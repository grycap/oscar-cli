package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestServiceRunCommandTextInput(t *testing.T) {
	const (
		clusterName  = "run-cluster"
		serviceName  = "echo"
		serviceToken = "service-token"
		payload      = "ping"
	)

	var (
		receivedBody string
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
		case r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading run payload: %v", err)
			}
			receivedBody = strings.TrimSpace(string(body))
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("RUN OK")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	stdout, stderr, err := runCommand(t,
		"service", "--config", configFile,
		"run", serviceName,
		"--cluster", clusterName,
		"--text-input", payload,
	)
	if err != nil {
		t.Fatalf("service run command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if strings.TrimSpace(stdout) != "RUN OK" {
		t.Fatalf("expected RUN OK output, got %q", stdout)
	}

	decoded, err := base64.StdEncoding.DecodeString(receivedBody)
	if err != nil {
		t.Fatalf("decoding request payload: %v", err)
	}
	if string(decoded) != payload {
		t.Fatalf("expected payload %q, got %q", payload, decoded)
	}

}

func TestServiceRunCommandFileInput(t *testing.T) {
	const (
		clusterName  = "run-file-cluster"
		serviceName  = "file-processor"
		serviceToken = "file-token"
	)

	var (
		receivedBody string
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
		case r.Method == http.MethodPost && r.URL.Path == "/run/"+serviceName:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading run payload: %v", err)
			}
			receivedBody = strings.TrimSpace(string(body))
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, base64.StdEncoding.EncodeToString([]byte("FILE OK")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, clusterName, server.URL)

	fileDir := t.TempDir()
	inputFile := filepath.Join(fileDir, "payload.txt")
	expectedContent := "payload from file"
	if err := os.WriteFile(inputFile, []byte(expectedContent), 0o600); err != nil {
		t.Fatalf("writing input file: %v", err)
	}

	stdout, stderr, err := runCommand(t,
		"service", "--config", configFile,
		"run", serviceName,
		"--cluster", clusterName,
		"--file-input", inputFile,
	)
	if err != nil {
		t.Fatalf("service run command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if strings.TrimSpace(stdout) != "FILE OK" {
		t.Fatalf("expected FILE OK output, got %q", stdout)
	}

	decoded, err := base64.StdEncoding.DecodeString(receivedBody)
	if err != nil {
		t.Fatalf("decoding request payload: %v", err)
	}
	if string(decoded) != expectedContent {
		t.Fatalf("expected payload %q, got %q", expectedContent, decoded)
	}
}

func TestServiceRunCommandInputValidation(t *testing.T) {
	const clusterName = "run-validate-cluster"

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
				"run", "svc",
				"--cluster", clusterName,
			},
			wantErr: `you must specify "--file-input" or "--text-input" flag`,
		},
		{
			name: "conflicting inputs",
			args: []string{
				"service", "--config", configFile,
				"run", "svc",
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
				"run", "svc",
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
				"run", "svc",
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
