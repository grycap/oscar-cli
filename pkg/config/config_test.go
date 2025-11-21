package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
)

func TestReadConfigYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := `oscar:
  alpha:
    endpoint: "https://alpha"
    auth_user: "user"
    auth_password: "pass"
    ssl_verify: true
    memory: 256Mi
    log_level: INFO
default: alpha
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	conf, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("ReadConfig returned error: %v", err)
	}
	if conf.Default != "alpha" {
		t.Fatalf("expected default alpha, got %s", conf.Default)
	}
	if _, ok := conf.Oscar["alpha"]; !ok {
		t.Fatalf("expected cluster alpha in config")
	}
}

func TestReadConfigMissingFile(t *testing.T) {
	_, err := ReadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if err != errNoConfigFile {
		t.Fatalf("expected errNoConfigFile, got %v", err)
	}
}

func TestConfigAddAndRemoveCluster(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	conf := &Config{Oscar: map[string]*cluster.Cluster{}}

	if err := conf.AddCluster(configPath, "alpha", "https://alpha", "user", "pass", "", "", true); err != nil {
		t.Fatalf("AddCluster returned error: %v", err)
	}
	if conf.Default != "alpha" {
		t.Fatalf("expected default alpha after first addition, got %s", conf.Default)
	}
	if _, ok := conf.Oscar["alpha"]; !ok {
		t.Fatalf("alpha cluster not stored in config")
	}

	if err := conf.AddCluster(configPath, "beta", "https://beta", "", "", "", "", false); err != nil {
		t.Fatalf("AddCluster beta returned error: %v", err)
	}
	if conf.Default != "alpha" {
		t.Fatalf("expected default alpha to remain, got %s", conf.Default)
	}

	if err := conf.SetDefault(configPath, "beta"); err != nil {
		t.Fatalf("SetDefault returned error: %v", err)
	}
	if conf.Default != "beta" {
		t.Fatalf("expected default beta, got %s", conf.Default)
	}

	if err := conf.RemoveCluster(configPath, "beta"); err != nil {
		t.Fatalf("RemoveCluster returned error: %v", err)
	}
	if conf.Default != "" {
		t.Fatalf("expected default cleared after removing default cluster, got %s", conf.Default)
	}
	if _, exists := conf.Oscar["beta"]; exists {
		t.Fatalf("expected beta cluster to be removed")
	}
}

func TestGetClusterResolution(t *testing.T) {
	conf := &Config{
		Oscar: map[string]*cluster.Cluster{
			"alpha": {Endpoint: "https://alpha"},
			"beta":  {Endpoint: "https://beta"},
		},
		Default: "alpha",
	}

	t.Run("default flag", func(t *testing.T) {
		got, err := conf.GetCluster(true, "", "")
		if err != nil {
			t.Fatalf("GetCluster returned error: %v", err)
		}
		if got != "alpha" {
			t.Fatalf("expected alpha, got %s", got)
		}
	})

	t.Run("destination flag", func(t *testing.T) {
		got, err := conf.GetCluster(false, "beta", "")
		if err != nil {
			t.Fatalf("GetCluster returned error: %v", err)
		}
		if got != "beta" {
			t.Fatalf("expected beta, got %s", got)
		}
	})

	t.Run("default cluster name constant", func(t *testing.T) {
		got, err := conf.GetCluster(false, "", defaultClusterName)
		if err != nil {
			t.Fatalf("GetCluster returned error: %v", err)
		}
		if got != "alpha" {
			t.Fatalf("expected alpha, got %s", got)
		}
	})

	t.Run("explicit cluster", func(t *testing.T) {
		got, err := conf.GetCluster(false, "", "beta")
		if err != nil {
			t.Fatalf("GetCluster returned error: %v", err)
		}
		if got != "beta" {
			t.Fatalf("expected beta, got %s", got)
		}
	})

	t.Run("missing cluster", func(t *testing.T) {
		_, err := conf.GetCluster(false, "", "gamma")
		if err == nil {
			t.Fatalf("expected error for missing cluster")
		}
	})
}
