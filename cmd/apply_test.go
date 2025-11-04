package cmd

import (
	"testing"

	"github.com/grycap/oscar/v3/pkg/types"
)

func TestOverrideServiceNameUpdatesPaths(t *testing.T) {
	svc := &types.Service{
		Name: "demo",
		Input: []types.StorageIOConfig{
			{Path: "demo/in"},
			{Path: "other/in"},
		},
		Output: []types.StorageIOConfig{{Path: "demo"}},
		Mount:  types.StorageIOConfig{Path: "demo/mount"},
	}

	overrideServiceName(svc, "demo-new")

	if svc.Name != "demo-new" {
		t.Fatalf("expected service name demo-new, got %s", svc.Name)
	}
	if got := svc.Input[0].Path; got != "demo-new/in" {
		t.Fatalf("expected first input path demo-new/in, got %s", got)
	}
	if got := svc.Input[1].Path; got != "other/in" {
		t.Fatalf("unexpected path rewrite: %s", got)
	}
	if got := svc.Output[0].Path; got != "demo-new" {
		t.Fatalf("expected output path demo-new, got %s", got)
	}
	if got := svc.Mount.Path; got != "demo-new/mount" {
		t.Fatalf("expected mount path demo-new/mount, got %s", got)
	}
}

func TestReplacePathBucket(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		old      string
		new      string
		expected string
	}{
		{"simple", "demo", "demo", "new", "new"},
		{"withDir", "demo/files", "demo", "new", "new/files"},
		{"unmatched", "other", "demo", "new", "other"},
		{"leadingSlash", "/demo/files", "demo", "new", "/new/files"},
		{"trailingSlash", "demo/", "demo", "new", "new/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replacePathBucket(tc.path, tc.old, tc.new)
			if got != tc.expected {
				t.Fatalf("replacePathBucket(%q) = %q, want %q", tc.path, got, tc.expected)
			}
		})
	}
}
