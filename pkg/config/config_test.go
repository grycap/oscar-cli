package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
)

func TestReadConfigPreservesClusterOrderYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `oscar:
  alpha:
    endpoint: http://alpha
  beta:
    endpoint: http://beta
  gamma:
    endpoint: http://gamma
default: alpha
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ReadConfig(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	got := cfg.ClusterIDs()
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

func TestClusterIDsAlphabeticalFallback(t *testing.T) {
	cfg := &Config{
		Oscar: map[string]*cluster.Cluster{
			"delta": {},
			"beta":  {},
			"alpha": {},
		},
	}

	got := cfg.ClusterIDs()
	want := []string{"alpha", "beta", "delta"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %s, want %s", i, got[i], want[i])
		}
	}
}
