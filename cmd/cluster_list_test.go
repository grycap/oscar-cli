package cmd

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestClusterListCommandPrintsClusters(t *testing.T) {
	const configContent = `oscar:
  alpha:
    endpoint: "https://alpha"
    auth_user: ""
    auth_password: ""
    ssl_verify: false
    memory: 256Mi
    log_level: INFO
  beta:
    endpoint: "https://beta"
    auth_user: ""
    auth_password: ""
    ssl_verify: false
    memory: 256Mi
    log_level: INFO
default: beta
`

	configFile := writeRawConfig(t, configContent)

	originalNoColor := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = originalNoColor }()

	stdout, stderr, err := runCommand(t,
		"cluster", "--config", configFile,
		"list",
	)
	if err != nil {
		t.Fatalf("cluster list command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "alpha (https://alpha)") {
		t.Fatalf("expected alpha entry, got %q", stdout)
	}
	if !strings.Contains(stdout, "beta (https://beta) (Default)") {
		t.Fatalf("expected default beta entry, got %q", stdout)
	}
}

func TestClusterListCommandNoClusters(t *testing.T) {
	const configContent = `oscar: {}
default: ""
`

	configFile := writeRawConfig(t, configContent)

	originalNoColor := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = originalNoColor }()

	stdout, stderr, err := runCommand(t,
		"cluster", "--config", configFile,
		"list",
	)
	if err != nil {
		t.Fatalf("cluster list command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "There are no defined clusters in the config file") {
		t.Fatalf("expected empty clusters message, got %q", stdout)
	}
}
