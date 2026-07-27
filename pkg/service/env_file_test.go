package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grycap/oscar/v4/pkg/types"
)

func TestReadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `
# comment
OPENAI_API_KEY=secret
OPENAI_MODEL="agentic"
export OPENAI_BASE_URL=https://llm.ai.egi.eu/v1
IGNORED='value with spaces'
`
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing env file: %v", err)
	}

	values, err := ReadEnvFile(envPath)
	if err != nil {
		t.Fatalf("ReadEnvFile returned error: %v", err)
	}

	if got := values["OPENAI_API_KEY"]; got != "secret" {
		t.Fatalf("expected OPENAI_API_KEY secret, got %q", got)
	}
	if got := values["OPENAI_MODEL"]; got != "agentic" {
		t.Fatalf("expected OPENAI_MODEL agentic, got %q", got)
	}
	if got := values["OPENAI_BASE_URL"]; got != "https://llm.ai.egi.eu/v1" {
		t.Fatalf("expected OPENAI_BASE_URL URL, got %q", got)
	}
	if got := values["IGNORED"]; got != "value with spaces" {
		t.Fatalf("expected quoted value with spaces, got %q", got)
	}
}

func TestApplyEnvFileValuesToServiceOverridesDeclaredKeysOnly(t *testing.T) {
	svc := &types.Service{}
	svc.Environment.Vars = map[string]string{
		"OPENAI_BASE_URL": "old-url",
		"OPENAI_MODEL":    "old-model",
	}
	svc.Environment.Secrets = map[string]string{
		"OPENAI_API_KEY": "old-secret",
	}

	ApplyEnvFileValuesToService(svc, map[string]string{
		"OPENAI_API_KEY":  "new-secret",
		"OPENAI_BASE_URL": "new-url",
		"OPENAI_MODEL":    "new-model",
		"OTHER":           "ignored",
	})

	if got := svc.Environment.Secrets["OPENAI_API_KEY"]; got != "new-secret" {
		t.Fatalf("expected OPENAI_API_KEY override, got %q", got)
	}
	if got := svc.Environment.Vars["OPENAI_BASE_URL"]; got != "new-url" {
		t.Fatalf("expected OPENAI_BASE_URL override, got %q", got)
	}
	if got := svc.Environment.Vars["OPENAI_MODEL"]; got != "new-model" {
		t.Fatalf("expected OPENAI_MODEL override, got %q", got)
	}
	if _, ok := svc.Environment.Vars["OTHER"]; ok {
		t.Fatal("expected undeclared env key to be ignored")
	}
	if _, ok := svc.Environment.Secrets["OTHER"]; ok {
		t.Fatal("expected undeclared secret key to be ignored")
	}
}
