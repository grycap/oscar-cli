package hub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grycap/oscar-cli/v2/pkg/cluster"
	"github.com/grycap/oscar-cli/v2/pkg/storage"
	"github.com/grycap/oscar/v4/pkg/types"
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

func TestParseAcceptanceCommandHTTP(t *testing.T) {
	cmd, err := parseAcceptanceCommand("oscar-cli service http demo --method POST --path /v2/models/demo/predict/ --accept application/zip --form-field data --status 200")
	if err != nil {
		t.Fatalf("parseAcceptanceCommand returned error: %v", err)
	}

	if cmd.Kind != stepCommandHTTP {
		t.Fatalf("expected stepCommandHTTP, got %v", cmd.Kind)
	}
	if cmd.ServiceName != "demo" {
		t.Fatalf("expected service name demo, got %s", cmd.ServiceName)
	}
	if cmd.HTTPMethod != http.MethodPost {
		t.Fatalf("expected POST method, got %s", cmd.HTTPMethod)
	}
	if cmd.HTTPPath != "/v2/models/demo/predict/" {
		t.Fatalf("unexpected http path %q", cmd.HTTPPath)
	}
	if cmd.HTTPAccept != "application/zip" {
		t.Fatalf("unexpected accept header %q", cmd.HTTPAccept)
	}
	if cmd.HTTPFormField != "data" {
		t.Fatalf("unexpected form field %q", cmd.HTTPFormField)
	}
	if cmd.HTTPExpectStatus != http.StatusOK {
		t.Fatalf("unexpected expected status %d", cmd.HTTPExpectStatus)
	}
	if !cmd.HTTPUseServiceAuth {
		t.Fatalf("expected service auth to be enabled by default")
	}
}

func TestInvokeExposedHTTPUsesServiceAuthAndMultipart(t *testing.T) {
	const (
		serviceName  = "demo"
		serviceToken = "svc-token"
	)

	zipPayload := buildTestZIP(t)
	var seenAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/system/services/demo/exposed/v2/models/demo/predict/" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Fatalf("expected basic auth header")
		}
		seenAuthHeader = r.Header.Get("Authorization")
		if user != serviceName || pass != serviceToken {
			t.Fatalf("unexpected credentials %s:%s", user, pass)
		}
		if got := r.Header.Get("Accept"); got != "application/zip" {
			t.Fatalf("unexpected accept header %q", got)
		}
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parsing content type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type %q", mediaType)
		}
		reader := multipartReader(t, r.Body, params["boundary"])
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("reading multipart part: %v", err)
		}
		if part.FormName() != "data" {
			t.Fatalf("unexpected form field %q", part.FormName())
		}
		if part.FileName() != "001.jpg" {
			t.Fatalf("unexpected file name %q", part.FileName())
		}
		if got := part.Header.Get("Content-Type"); got != "image/jpeg" {
			t.Fatalf("unexpected part content type %q", got)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("reading part body: %v", err)
		}
		if string(body) != "image-bytes" {
			t.Fatalf("unexpected multipart body %q", string(body))
		}

		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(zipPayload); err != nil {
			t.Fatalf("writing response: %v", err)
		}
	}))
	defer server.Close()

	data, mediaType, status, err := invokeExposedHTTP(
		&cluster.Cluster{Endpoint: server.URL, SSLVerify: true},
		&types.Service{Name: serviceName, Token: serviceToken, Expose: types.Expose{SetAuth: true}},
		serviceName,
		parsedCommand{
			Kind:               stepCommandHTTP,
			HTTPMethod:         http.MethodPost,
			HTTPPath:           "/v2/models/demo/predict/",
			HTTPAccept:         "application/zip",
			HTTPFormField:      "data",
			HTTPUseServiceAuth: true,
			HTTPExpectStatus:   http.StatusOK,
		},
		[]httpPayload{{Name: "001.jpg", Content: []byte("image-bytes"), ContentType: "image/jpeg"}},
	)
	if err != nil {
		t.Fatalf("invokeExposedHTTP returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if mediaType != "application/zip" {
		t.Fatalf("unexpected response media type %q", mediaType)
	}
	if !bytes.Equal(data, zipPayload) {
		t.Fatalf("unexpected response payload")
	}
	if !strings.HasPrefix(seenAuthHeader, "Basic ") {
		t.Fatalf("expected basic authorization header, got %q", seenAuthHeader)
	}
	if err := validateZIPPayload(data); err != nil {
		t.Fatalf("validateZIPPayload returned error: %v", err)
	}
}

func TestAcceptanceCommandsRenderHTTPAndLocalPaths(t *testing.T) {
	dir := t.TempDir()
	crateDir := filepath.Join(dir, "posenet-tf")
	if err := os.MkdirAll(crateDir, 0o755); err != nil {
		t.Fatalf("creating crate dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "001.jpg"), []byte("jpeg"), 0o600); err != nil {
		t.Fatalf("writing sample image: %v", err)
	}
	raw := `{
	  "@graph": [
	    { "@id": "./", "subjectOf": [{ "@id": "#acceptance" }] },
	    { "@id": "001.jpg", "@type": ["File","ImageObject"], "encodingFormat": "image/jpeg" },
	    { "@id": "#expected-zip", "@type": ["File"], "encodingFormat": "application/zip" },
	    {
	      "@id": "#acceptance",
	      "@type": "HowTo",
	      "name": "HTTP acceptance",
	      "step": [{ "@id": "#step-http" }]
	    },
	    {
	      "@id": "#step-http",
	      "@type": "HowToStep",
	      "position": 1,
	      "potentialAction": { "@id": "#action-http" }
	    },
	    {
	      "@id": "#action-http",
	      "@type": "ConsumeAction",
	      "name": "http-request",
	      "object": { "@id": "001.jpg" },
	      "result": { "@id": "#expected-zip" },
	      "additionalProperty": [
	        { "@id": "#prop-method" },
	        { "@id": "#prop-path" },
	        { "@id": "#prop-accept" },
	        { "@id": "#prop-form-field" }
	      ]
	    },
	    { "@id": "#prop-method", "@type": "PropertyValue", "propertyID": "method", "value": "POST" },
	    { "@id": "#prop-path", "@type": "PropertyValue", "propertyID": "path", "value": "/v2/models/posenetclas/predict/" },
	    { "@id": "#prop-accept", "@type": "PropertyValue", "propertyID": "accept", "value": "application/zip" },
	    { "@id": "#prop-form-field", "@type": "PropertyValue", "propertyID": "formField", "value": "data" }
	  ]
	}`
	if err := os.WriteFile(filepath.Join(crateDir, "ro-crate-metadata.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("writing ro-crate: %v", err)
	}

	client := NewClient()
	sets, err := client.AcceptanceCommands(context.Background(), "posenet-tf", "body-pose", dir)
	if err != nil {
		t.Fatalf("AcceptanceCommands returned error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 command set, got %d", len(sets))
	}
	if len(sets[0].Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(sets[0].Commands))
	}
	command := sets[0].Commands[0]
	if !strings.Contains(command, "curl") {
		t.Fatalf("expected curl command, got %q", command)
	}
	if !strings.Contains(command, "body-pose:${SERVICE_TOKEN}") {
		t.Fatalf("expected service auth placeholder, got %q", command)
	}
	if !strings.Contains(command, "/system/services/body-pose/exposed/v2/models/posenetclas/predict/") {
		t.Fatalf("expected exposed path, got %q", command)
	}
	if !strings.Contains(command, "--output './acceptance-output.zip'") && !strings.Contains(command, "--output ./acceptance-output.zip") {
		t.Fatalf("expected output redirection, got %q", command)
	}
	absImage, _ := filepath.Abs(filepath.Join(crateDir, "001.jpg"))
	if !strings.Contains(command, absImage) {
		t.Fatalf("expected absolute sample image path, got %q", command)
	}
}

func multipartReader(t *testing.T, body io.Reader, boundary string) *multipart.Reader {
	t.Helper()
	return multipart.NewReader(body, boundary)
}

func buildTestZIP(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("result.txt")
	if err != nil {
		t.Fatalf("creating zip entry: %v", err)
	}
	if _, err := entry.Write([]byte("ok")); err != nil {
		t.Fatalf("writing zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}
	return buffer.Bytes()
}
