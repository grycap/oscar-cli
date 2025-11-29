package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestDefaultIfEmpty(t *testing.T) {
	if got := defaultIfEmpty(" value ", "fallback"); got != " value " {
		t.Fatalf("expected original value, got %q", got)
	}
	if got := defaultIfEmpty("   ", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := maskSecret("secret"); got != "******" {
		t.Fatalf("expected masked secret, got %q", got)
	}
	long := strings.Repeat("x", 32)
	if got := maskSecret(long); got != strings.Repeat("*", 8) {
		t.Fatalf("expected capped mask, got %q", got)
	}
}

func TestTrimToken(t *testing.T) {
	token := strings.Repeat("a", 70) + "\nsecond line"
	got := trimToken(token)
	if len(got) != 64 {
		t.Fatalf("expected trimmed token length 64, got %d", len(got))
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("expected first line only, got %q", got)
	}
}

func TestBucketVisibilityColor(t *testing.T) {
	tests := []struct {
		value string
		color tcell.Color
	}{
		{"restricted", tcell.ColorYellow},
		{"private", tcell.ColorRed},
		{"public", tcell.ColorGreen},
		{"", tcell.ColorWhite},
	}
	for _, tt := range tests {
		if got := bucketVisibilityColor(tt.value); got != tt.color {
			t.Fatalf("bucketVisibilityColor(%q) = %v, want %v", tt.value, got, tt.color)
		}
	}
}
