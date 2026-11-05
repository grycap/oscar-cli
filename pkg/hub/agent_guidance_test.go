package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/grycap/oscar/v4/pkg/types"
)

func agentFixture(soul string, skills ...string) []byte {
	parts := []any{map[string]string{"@id": soul}}
	graph := []any{map[string]any{
		"@id": "./", "@type": []string{"Dataset", "Agent"},
		"conformsTo":  map[string]string{"@id": "https://github.com/grycap/oscar-agents/tree/devel/profiles/oscar-agent-1.1"},
		"agentSoul":   map[string]string{"@id": soul},
		"agentSkills": []any{}, "hasPart": parts,
	}}
	graph = append(graph, map[string]any{"@id": soul, "@type": []string{"File", "AgentSoul"}, "encodingFormat": "text/markdown"})
	refs := []any{}
	for _, id := range skills {
		parts = append(parts, map[string]string{"@id": id})
		refs = append(refs, map[string]string{"@id": id})
		graph = append(graph, map[string]any{"@id": id, "@type": []string{"File", "AgentSkill"}, "encodingFormat": "text/markdown"})
	}
	graph[0].(map[string]any)["hasPart"] = parts
	graph[0].(map[string]any)["agentSkills"] = refs
	raw, _ := json.Marshal(map[string]any{"@graph": graph})
	return raw
}

func agentFDL() *service.FDL {
	fdl := &service.FDL{}
	fdl.Functions.Oscar = []map[string]*types.Service{{"default": {}}}
	fdl.Functions.Oscar[0]["default"].Environment.Vars = map[string]string{"AGENT_SOUL": "YOUR_AGENT_SOUL", "AGENT_SKILLS": "YOUR_AGENT_SKILLS"}
	return fdl
}

func TestMaterializeAgentGuidance(t *testing.T) {
	fdl := agentFDL()
	files := map[string]string{"SOUL.md": "soul\n", "skills/z/SKILL.md": "z\n", "skills/a/SKILL.md": "a\n"}
	err := MaterializeAgentGuidance(fdl, agentFixture("SOUL.md", "skills/z/SKILL.md", "skills/a/SKILL.md"), func(id string) ([]byte, error) {
		return []byte(files[id]), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	vars := fdl.Functions.Oscar[0]["default"].Environment.Vars
	if vars["AGENT_SOUL"] != "soul\n" || vars["AGENT_SKILLS"] != "a\n\n\nz\n" {
		t.Fatalf("unexpected materialization: %#v", vars)
	}
}

func TestMaterializeAgentGuidanceRejectsInvalidReference(t *testing.T) {
	for _, id := range []string{"../SOUL.md", "/SOUL.md", "https://host/SOUL.md", "skills/../SOUL.md", "SOUL.txt"} {
		t.Run(id, func(t *testing.T) {
			called := false
			err := MaterializeAgentGuidance(agentFDL(), agentFixture(id), func(string) ([]byte, error) {
				called = true
				return []byte("data"), nil
			})
			if err == nil || called {
				t.Fatalf("expected rejection before reading %s; err=%v read=%t", id, err, called)
			}
		})
	}
}

func TestMaterializeAgentGuidanceDoesNotInjectUndeclared(t *testing.T) {
	fdl := agentFDL()
	fdl.Functions.Oscar[0]["default"].Environment.Vars = nil
	called := false
	err := MaterializeAgentGuidance(fdl, agentFixture("SOUL.md"), func(string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if err != nil || called || fdl.Functions.Oscar[0]["default"].Environment.Vars != nil {
		t.Fatalf("unexpected undeclared injection: %v", err)
	}
}

func TestMaterializeAgentGuidanceRejectsMissingOrNonUTF8File(t *testing.T) {
	metadata := agentFixture("SOUL.md")
	for _, tc := range []struct {
		name string
		data []byte
		err  error
	}{
		{"missing", nil, os.ErrNotExist},
		{"non-utf8", []byte{0xff}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fdl := agentFDL()
			err := MaterializeAgentGuidance(fdl, metadata, func(string) ([]byte, error) { return tc.data, tc.err })
			if err == nil {
				t.Fatal("expected invalid guidance to fail")
			}
			if fdl.Functions.Oscar[0]["default"].Environment.Vars["AGENT_SOUL"] != "YOUR_AGENT_SOUL" {
				t.Fatal("failed resolution changed the FDL")
			}
		})
	}
}

func TestMaterializeLocalAgentGuidanceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "SOUL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ro-crate-metadata.json"), agentFixture("SOUL.md"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MaterializeLocalAgentGuidance(root, "example", agentFDL()); err == nil || !strings.Contains(err.Error(), "escapes crate") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestMaterializeRemoteAgentGuidance(t *testing.T) {
	metadata := agentFixture("SOUL.md", "skills/a/SKILL.md")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "devel" {
			t.Errorf("unexpected ref %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/repos/grycap/oscar-agents/contents/crates/demo/ro-crate-metadata.json":
			_, _ = w.Write(metadata)
		case "/repos/grycap/oscar-agents/contents/crates/demo/SOUL.md":
			fmt.Fprint(w, "remote soul\n")
		case "/repos/grycap/oscar-agents/contents/crates/demo/skills/a/SKILL.md":
			fmt.Fprint(w, "remote skill\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(WithOwner("grycap"), WithRepo("oscar-agents"), WithRootPath("crates"), WithRef("devel"), WithBaseAPI(server.URL))
	fdl := agentFDL()
	if err := client.MaterializeRemoteAgentGuidance(context.Background(), "demo", fdl); err != nil {
		t.Fatal(err)
	}
	vars := fdl.Functions.Oscar[0]["default"].Environment.Vars
	if vars["AGENT_SOUL"] != "remote soul\n" || vars["AGENT_SKILLS"] != "remote skill\n" {
		t.Fatalf("unexpected remote values: %#v", vars)
	}
}
