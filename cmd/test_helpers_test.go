package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/fatih/color"
)

func runCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	root := NewRootCommand()
	root.SetArgs(args)

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stderr pipe: %v", err)
	}

	defer stdoutReader.Close()
	defer stderrReader.Close()

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	originalColorOut := color.Output
	originalColorErr := color.Error

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	color.Output = stdoutWriter
	color.Error = stderrWriter
	root.SetOut(stdoutWriter)
	root.SetErr(stderrWriter)

	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&outBuf, stdoutReader)
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&errBuf, stderrReader)
	}()

	_, execErr := root.ExecuteC()

	stdoutWriter.Close()
	stderrWriter.Close()
	wg.Wait()

	os.Stdout = originalStdout
	os.Stderr = originalStderr
	color.Output = originalColorOut
	color.Error = originalColorErr

	return outBuf.String(), errBuf.String(), execErr
}

func writeConfigFile(t *testing.T, clusterName, endpoint string) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := fmt.Sprintf(`oscar:
  %s:
    endpoint: "%s"
    auth_user: "user"
    auth_password: "pass"
    ssl_verify: false
    memory: 256Mi
    log_level: INFO
default: %s
`, clusterName, endpoint, clusterName)

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	return configPath
}

func writeRawConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing raw config: %v", err)
	}
	return configPath
}
