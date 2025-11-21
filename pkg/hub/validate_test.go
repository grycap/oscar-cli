package hub

import (
	"testing"

	"github.com/grycap/oscar-cli/pkg/storage"
)

func TestParseAcceptanceCommandRun(t *testing.T) {
	cmd, err := parseAcceptanceCommand("oscar-cli service run demo -i payload")
	if err != nil {
		t.Fatalf("parseAcceptanceCommand returned error: %v", err)
	}

	if cmd.Kind != stepCommandRun {
		t.Fatalf("expected stepCommandRun, got %v", cmd.Kind)
	}

	if cmd.ServiceName != "demo" {
		t.Fatalf("expected service name demo, got %s", cmd.ServiceName)
	}

	if cmd.RunDirective.Mode != inputModeText {
		t.Fatalf("expected text input mode, got %v", cmd.RunDirective.Mode)
	}

	if cmd.RunDirective.Value != "payload" {
		t.Fatalf("expected directive value payload, got %s", cmd.RunDirective.Value)
	}
}

func TestParseAcceptanceCommandPutFile(t *testing.T) {
	cmd, err := parseAcceptanceCommand("oscar-cli service put-file demo input.txt")
	if err != nil {
		t.Fatalf("parseAcceptanceCommand returned error: %v", err)
	}

	if cmd.Kind != stepCommandPutFile {
		t.Fatalf("expected stepCommandPutFile, got %v", cmd.Kind)
	}

	if cmd.ServiceName != "demo" {
		t.Fatalf("expected service name demo, got %s", cmd.ServiceName)
	}

	if cmd.LocalPath != "input.txt" {
		t.Fatalf("expected local path input.txt, got %s", cmd.LocalPath)
	}

	if cmd.Provider != storage.DefaultStorageProvider[0] {
		t.Fatalf("unexpected provider %s", cmd.Provider)
	}
}

func TestParseAcceptanceCommandGetFileLatest(t *testing.T) {
	cmd, err := parseAcceptanceCommand("ocli-dev service get-file demo --download-latest-into=out.txt")
	if err != nil {
		t.Fatalf("parseAcceptanceCommand returned error: %v", err)
	}

	if cmd.Kind != stepCommandGetFile {
		t.Fatalf("expected stepCommandGetFile, got %v", cmd.Kind)
	}

	if cmd.ServiceName != "demo" {
		t.Fatalf("expected service name demo, got %s", cmd.ServiceName)
	}

	if !cmd.LatestRequested {
		t.Fatalf("expected LatestRequested to be true")
	}

	if cmd.LatestValue != "out.txt" {
		t.Fatalf("expected LatestValue out.txt, got %s", cmd.LatestValue)
	}

	if cmd.LocalProvided {
		t.Fatalf("expected LocalProvided to be false when destination derived from flag")
	}
}
