package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar/v4/pkg/types"
)

func TestHubAgentDeploySendsMaterializedGuidance(t *testing.T) {
	const metadata = `{"@graph":[{"@id":"./","@type":["Dataset","Agent"],"conformsTo":{"@id":"https://github.com/grycap/oscar-agents/tree/devel/profiles/oscar-agent-1.1"},"agentSoul":{"@id":"SOUL.md"},"agentSkills":[{"@id":"skills/a/SKILL.md"}],"hasPart":[{"@id":"SOUL.md"},{"@id":"skills/a/SKILL.md"}]},{"@id":"SOUL.md","@type":["File","AgentSoul"],"encodingFormat":"text/markdown"},{"@id":"skills/a/SKILL.md","@type":["File","AgentSkill"],"encodingFormat":"text/markdown"}]}`
	const fdl = `functions:
  oscar:
  - default:
      name: agent-demo
      script: script.sh
      environment:
        variables:
          AGENT_SOUL: YOUR_AGENT_SOUL
          AGENT_SKILLS: YOUR_AGENT_SKILLS
`
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "devel" {
			t.Errorf("unexpected Git ref: %q", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo":
			fmt.Fprint(w, `[{"name":"fdl.yml","type":"file"},{"name":"script.sh","type":"file"}]`)
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo/fdl.yml":
			fmt.Fprint(w, fdl)
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo/script.sh":
			fmt.Fprint(w, "#!/bin/sh\n")
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo/ro-crate-metadata.json":
			fmt.Fprint(w, metadata)
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo/SOUL.md":
			fmt.Fprint(w, "agent identity\n")
		case "/repos/grycap/oscar-agents/contents/crates/agent-demo/skills/a/SKILL.md":
			fmt.Fprint(w, "agent ability\n")
		default:
			t.Errorf("unexpected GitHub request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer gitServer.Close()

	var deployed types.Service
	cluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/system/config":
			fmt.Fprint(w, `{"name":"oscar","namespace":"oscar","services_namespace":"oscar-svc"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/system/services/agent-demo":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/system/services":
			if err := json.NewDecoder(r.Body).Decode(&deployed); err != nil {
				t.Errorf("decoding payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected OSCAR request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer cluster.Close()

	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("oscar:\n  test:\n    endpoint: %s\ndefault: test\n", cluster.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	oldConfig := configPath
	configPath = config
	t.Cleanup(func() { configPath = oldConfig })
	cmd := makeHubDeployCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"agent-demo", "--repo", "oscar-agents", "--ref", "devel", "--api-base", gitServer.URL, "--cluster", "test"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if deployed.Environment.Vars["AGENT_SOUL"] != "agent identity\n" || deployed.Environment.Vars["AGENT_SKILLS"] != "agent ability\n" || !strings.Contains(deployed.Script, "#!/bin/sh") {
		t.Fatalf("deployment payload missing resolved guidance/script: %#v", deployed.Environment.Vars)
	}
}
