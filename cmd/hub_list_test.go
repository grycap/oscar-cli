package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHubListCommandOutputsTable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/grycap/oscar-hub/contents":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"name":"cowsay","path":"cowsay","type":"dir"},
				{"name":"broken","path":"broken","type":"dir"}
			]`))
		case "/repos/grycap/oscar-hub/contents/cowsay/ro-crate-metadata.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(cliSampleROCrate("Cowsay", "OSCAR Team")))
		case "/repos/grycap/oscar-hub/contents/broken/ro-crate-metadata.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{ invalid json"))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cmd := makeHubListCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--api-base", ts.URL})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hub list command returned error: %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two lines in output, got %q", output)
	}
	headerFields := strings.Fields(lines[0])
	if len(headerFields) != 3 || headerFields[0] != "SLUG" || headerFields[1] != "NAME" || headerFields[2] != "CREATOR" {
		t.Fatalf("unexpected header fields: %v", headerFields)
	}
	if !strings.Contains(lines[1], "cowsay") || !strings.Contains(lines[1], "Cowsay") || !strings.Contains(lines[1], "OSCAR Team") {
		t.Fatalf("unexpected row content: %q", lines[1])
	}
	if !strings.Contains(stderr.String(), "warning: broken") {
		t.Fatalf("expected warning for broken service, got %q", stderr.String())
	}
}

func cliSampleROCrate(name, creator string) string {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return `{
  "@context": "https://w3id.org/ro/crate/1.1/context",
  "@graph": [
    {
      "@id": "ro-crate-metadata.json",
      "@type": "CreativeWork",
      "about": { "@id": "./" }
    },
    {
      "@id": "./",
      "@type": "Dataset",
      "name": "` + name + `",
      "description": "Description for ` + name + `",
      "author": { "@id": "https://example.org/` + slug + `" }
    },
    {
      "@id": "https://example.org/` + slug + `",
      "@type": "Organization",
      "name": "` + creator + `"
    }
  ]
}`
}
