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

func TestRequiresEndpointOrToken(t *testing.T) {
	sets := []hub.AcceptanceCommandSet{
		{Commands: []string{"echo hello"}},
	}
	if requiresEndpointOrToken(sets) {
		t.Fatalf("expected false when commands do not use endpoint/token placeholders")
	}

	sets = []hub.AcceptanceCommandSet{
		{Commands: []string{"curl '${OSCAR_ENDPOINT%/}/system/services/demo/exposed' -u demo:${SERVICE_TOKEN}"}},
	}
	if !requiresEndpointOrToken(sets) {
		t.Fatalf("expected true when command uses endpoint/token placeholders")
	}
}
