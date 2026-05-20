package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/cmd"
)

func TestRootCommandHelp(t *testing.T) {
	root := cmd.NewRootCommand()
	root.SetArgs([]string{"--help"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if _, err := root.ExecuteC(); err != nil {
		t.Fatalf("root command --help returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "A CLI tool to interact with OSCAR clusters") {
		t.Fatalf("expected help text to mention OSCAR, got %q", output)
	}
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected help text to include Usage, got %q", output)
	}
}
