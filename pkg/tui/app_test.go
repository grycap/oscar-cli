package tui

import (
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{"shorter", "oscar", 10, "oscar"},
		{"exact", "oscar", 5, "oscar"},
		{"longer", "abcdefghijklmnopqrstuvwxyz", 5, "abcd…"},
		{"zero", "oscar", 0, "oscar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateString(tt.input, tt.limit); got != tt.want {
				t.Fatalf("truncateString(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}

func TestFormatServiceDetails(t *testing.T) {
	svc := &types.Service{
		Name:      "demo",
		ClusterID: "cluster-a",
		Image:     "ghcr.io/demo/service:latest",
		Memory:    "512Mi",
		CPU:       "0.5",
		LogLevel:  "INFO",
	}

	got := formatServiceDetails(svc)
	if got == "" {
		t.Fatal("formatServiceDetails returned empty string")
	}
	if want := "demo"; !containsString(got, want) {
		t.Fatalf("expected output to contain %q, got %q", want, got)
	}
	if want := "cluster-a"; !containsString(got, want) {
		t.Fatalf("expected output to contain %q, got %q", want, got)
	}
	if want := "ghcr.io/demo/service:latest"; !containsString(got, want) {
		t.Fatalf("expected output to contain %q, got %q", want, got)
	}
}

func TestFormatClusterConfig(t *testing.T) {
	cfg := &cluster.Cluster{
		Endpoint:         "https://example.test",
		AuthUser:         "admin",
		AuthPassword:     "supersecret",
		OIDCAccountName:  "oidc",
		OIDCRefreshToken: strings.Repeat("t", 80),
		SSLVerify:        true,
		Memory:           "256Mi",
		LogLevel:         "INFO",
	}

	text := formatClusterConfig("example", cfg)
	if strings.Contains(text, cfg.AuthPassword) {
		t.Fatalf("password should be obfuscated, got %q", text)
	}
	if !strings.Contains(text, "auth_password: ") {
		t.Fatalf("expected auth_password field")
	}
	if !strings.Contains(text, "ssl_verify: true") {
		t.Fatalf("expected ssl_verify field")
	}

	line := extractLine(text, "oidc_refresh_token")
	if len(line) == 0 {
		t.Fatalf("expected oidc_refresh_token line")
	}
	if len(strings.TrimSpace(line)) > len("oidc_refresh_token:")+1+64 {
		t.Fatalf("token line not trimmed: %q", line)
	}
}

func extractLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, prefix) {
			return line
		}
	}
	return ""
}

func containsString(haystack, needle string) bool {
	return len(needle) == 0 || strings.Contains(haystack, needle)
}
