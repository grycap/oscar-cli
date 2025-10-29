package tui

import (
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

func TestSortedClusters(t *testing.T) {
	input := map[string]*cluster.Cluster{
		"beta":  {},
		"alpha": {},
		"gamma": {},
	}

	got := sortedClusters(input)
	want := []string{"alpha", "beta", "gamma"}

	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedClusters mismatch at %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

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

func containsString(haystack, needle string) bool {
	return len(needle) == 0 || strings.Contains(haystack, needle)
}
