package cmd

import (
	"testing"

	"github.com/grycap/oscar-cli/v2/pkg/hub"
)

func TestMakeHubValidateCmdIncludesPrintAcceptanceCommandsFlag(t *testing.T) {
	cmd := makeHubValidateCmd()
	flag := cmd.Flags().Lookup("print-acceptance-commands")
	if flag == nil {
		t.Fatalf("expected print-acceptance-commands flag to be registered")
	}
}

func TestRequiresServiceToken(t *testing.T) {
	sets := []hub.AcceptanceCommandSet{
		{Commands: []string{"echo hello"}},
	}
	if requiresServiceToken(sets) {
		t.Fatalf("expected false when commands do not use a service token")
	}

	sets = []hub.AcceptanceCommandSet{
		{Commands: []string{"curl https://demo.example.org/v2 -u demo:${SERVICE_TOKEN}"}},
	}
	if !requiresServiceToken(sets) {
		t.Fatalf("expected true when command uses service token placeholder")
	}
}
