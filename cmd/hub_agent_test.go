package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHubAgentPreviewAndOverrideWithoutCluster(t *testing.T) {
	crate := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(crate, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"fdl.yml": `functions:
  oscar:
  - default:
      name: demo
      script: script.sh
      environment:
        variables:
          AGENT_SOUL: YOUR_AGENT_SOUL
          AGENT_SKILLS: YOUR_AGENT_SKILLS
        secrets:
          OPENAI_API_KEY: not-for-preview
      input:
      - storage_provider: minio.default
        path: demo/input
`,
		"script.sh":              "#!/bin/sh\n",
		"SOUL.md":                "soul\nline two\n",
		"ro-crate-metadata.json": `{"@graph":[{"@id":"./","@type":["Dataset","Agent"],"conformsTo":{"@id":"https://github.com/grycap/oscar-agents/tree/devel/profiles/oscar-agent-1.1"},"agentSoul":{"@id":"SOUL.md"},"hasPart":[{"@id":"SOUL.md"}]},{"@id":"SOUL.md","@type":["File","AgentSoul"],"encodingFormat":"text/markdown"}]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(crate, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("OPENAI_API_KEY=secret-override\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldConfig := configPath
	configPath = filepath.Join(t.TempDir(), "nonexistent-config.yaml")
	t.Cleanup(func() { configPath = oldConfig })
	cmd := makeHubDeployCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"demo", "--local-path", crate, "--preview", "--env-file", envFile, "--name", "renamed"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "AGENT_SOUL: |") || !strings.Contains(text, "script: |") || !strings.Contains(text, "line two\n") {
		t.Fatalf("expected multiline YAML in preview: %s", text)
	}
	if strings.Contains(text, "not-for-preview") || strings.Contains(text, "secret-override") || strings.Contains(text, "YOUR_AGENT_") {
		t.Fatal("preview leaked secrets or unresolved placeholders")
	}
	var preview map[string]any
	if err := yaml.Unmarshal(output.Bytes(), &preview); err != nil {
		t.Fatalf("preview is not valid YAML: %v", err)
	}
	entry := preview["functions"].(map[string]any)["oscar"].([]any)[0].(map[string]any)["default"].(map[string]any)
	if entry["name"] != "renamed" || entry["script"] != "#!/bin/sh\n" {
		t.Fatalf("preview does not represent effective service: %#v", entry)
	}
	if _, ok := entry["alpine"]; ok {
		t.Fatal("preview includes unconfigured zero-value service fields")
	}
	env := entry["environment"].(map[string]any)
	if env["variables"].(map[string]any)["AGENT_SOUL"] != "soul\nline two\n" || env["secrets"].(map[string]any)["OPENAI_API_KEY"] != "[REDACTED]" {
		t.Fatalf("unexpected guidance or secret handling: %#v", env)
	}
	if entry["input"].([]any)[0].(map[string]any)["path"] != "renamed/input" {
		t.Fatal("preview did not apply name override to storage paths")
	}
}

func TestHubPreviewRedactsCredentialFields(t *testing.T) {
	data := map[string]any{
		"environment": map[string]any{
			"variables": map[string]any{"API_KEY": "var-sensitive", "SAFE": "visible"},
			"secrets":   map[string]any{"GENERIC": "secret-sensitive"},
		},
		"clusters":          map[string]any{"example": map[string]any{"auth_password": "password-sensitive", "endpoint": "https://example.invalid"}},
		"storage_providers": map[string]any{"minio": map[string]any{"access_key": "access-sensitive", "secret_key": "key-sensitive"}},
	}
	redactPreview(data, "")
	encoded, err := yaml.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, value := range []string{"var-sensitive", "secret-sensitive", "password-sensitive", "access-sensitive", "key-sensitive"} {
		if strings.Contains(text, value) {
			t.Fatalf("credential leaked in preview: %s", value)
		}
	}
	if !strings.Contains(text, "https://example.invalid") || !strings.Contains(text, "visible") {
		t.Fatal("preview removed non-secret configuration")
	}
}

func TestHubAgentPreviewRejectsUnresolvedGuidance(t *testing.T) {
	crate := t.TempDir()
	if err := os.WriteFile(filepath.Join(crate, "fdl.yml"), []byte(`functions:
  oscar:
  - default:
      name: demo
      script: script.sh
      environment:
        variables:
          AGENT_SOUL: YOUR_AGENT_SOUL
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crate, "script.sh"), []byte("#!/bin/sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := makeHubDeployCmd()
	cmd.SetArgs([]string{"demo", "--local-path", crate, "--preview"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unresolved agent guidance") {
		t.Fatalf("expected unresolved guidance error; got %v", err)
	}
}
