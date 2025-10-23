package hub

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/service"
)

const (
	maxOutputPreview     = 512
	externalFetchTimeout = 30 * time.Second
)

var (
	errCommandMissingInput = errors.New("acceptance test command does not include a supported input flag")
)

type inputMode int

const (
	inputModeUnknown inputMode = iota
	inputModeFile
	inputModeText
)

type inputDirective struct {
	Mode  inputMode
	Value string
}

// ValidateService downloads the RO-Crate metadata for the provided slug, runs its acceptance tests against the cluster and returns the aggregated results.
func (c *Client) ValidateService(ctx context.Context, slug string, clusterCfg *cluster.Cluster, serviceNameOverride string) ([]AcceptanceResult, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("service slug cannot be empty")
	}
	if clusterCfg == nil {
		return nil, errors.New("cluster configuration is required")
	}

	repoPath := c.serviceRepoPath(slug)
	metadataPath := path.Join(repoPath, metadataFile)
	rawMetadata, err := c.getFile(ctx, metadataPath)
	if err != nil {
		return nil, err
	}

	crate, err := ParseROCrate(rawMetadata)
	if err != nil {
		return nil, err
	}

	tests, err := crate.AcceptanceTests()
	if err != nil {
		return nil, err
	}

	results := make([]AcceptanceResult, 0, len(tests))
	for _, test := range tests {
		res := AcceptanceResult{Test: test}
		output, runErr := c.executeAcceptanceTest(ctx, repoPath, slug, test, clusterCfg, serviceNameOverride)
		if runErr != nil {
			res.Err = runErr
			res.Passed = false
		} else {
			res.Output = previewOutput(output)
			if expected := strings.TrimSpace(test.ExpectedSubstring); expected != "" && !strings.Contains(output, expected) {
				res.Passed = false
				res.Details = fmt.Sprintf("expected substring %q not found", expected)
			} else {
				res.Passed = true
			}
		}
		results = append(results, res)
	}

	return results, nil
}

func (c *Client) executeAcceptanceTest(ctx context.Context, repoPath, slug string, test AcceptanceTest, clusterCfg *cluster.Cluster, serviceNameOverride string) (string, error) {
	directive, serviceName, err := parseAcceptanceCommand(test.Command)
	if err != nil {
		return "", fmt.Errorf("parsing command for test %s: %w", test.ID, err)
	}

	if strings.TrimSpace(serviceNameOverride) != "" {
		serviceName = serviceNameOverride
	}
	if serviceName == "" {
		serviceName = slug
	}

	supplyMap := make(map[string]TestInput, len(test.Inputs))
	for _, input := range test.Inputs {
		supplyMap[input.ID] = input
	}

	var payload []byte
	switch directive.Mode {
	case inputModeFile:
		input, ok := supplyMap[directive.Value]
		if !ok {
			return "", fmt.Errorf("input %q referenced in command not found in RO-Crate supply list", directive.Value)
		}
		payload, err = fetchSupplyContent(ctx, c, repoPath, input)
		if err != nil {
			return "", err
		}
	case inputModeText:
		if input, ok := supplyMap[directive.Value]; ok {
			payload, err = fetchSupplyContent(ctx, c, repoPath, input)
			if err != nil {
				return "", err
			}
		} else {
			payload = []byte(directive.Value)
		}
	default:
		return "", errCommandMissingInput
	}

	responseBytes, err := invokeServiceWithContent(clusterCfg, serviceName, payload)
	if err != nil {
		return "", err
	}

	return string(responseBytes), nil
}

func fetchSupplyContent(ctx context.Context, client *Client, repoPath string, input TestInput) ([]byte, error) {
	candidates := make([]string, 0, 2)
	if url := strings.TrimSpace(input.URL); url != "" {
		candidates = append(candidates, url)
	}
	if id := strings.TrimSpace(input.ID); id != "" {
		candidates = append(candidates, id)
	}

	var lastErr error
	for _, candidate := range candidates {
		if isAbsoluteURL(candidate) {
			data, err := downloadExternalResource(ctx, candidate)
			if err == nil {
				return data, nil
			}
			lastErr = err
			continue
		}

		data, err := readFromRepository(ctx, client, repoPath, candidate)
		if err == nil {
			return data, nil
		}
		// If the candidate was URL and failed due to escaping, propagate immediately.
		if errors.Is(err, errEscapesServiceDirectory) {
			return nil, err
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("unable to resolve input %q", input.ID)
}

var errEscapesServiceDirectory = errors.New("path escapes service directory")

func readFromRepository(ctx context.Context, client *Client, repoPath, relative string) ([]byte, error) {
	clean := path.Clean(strings.TrimSpace(relative))
	if clean == "." || clean == "" {
		return nil, fmt.Errorf("invalid input path %q", relative)
	}
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("%w: %s", errEscapesServiceDirectory, relative)
	}

	joined := path.Join(repoPath, clean)
	joined = strings.Trim(joined, "/")

	return client.getFile(ctx, joined)
}

func downloadExternalResource(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", rawURL, err)
	}

	client := &http.Client{Timeout: externalFetchTimeout}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", rawURL, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d downloading %s", res.StatusCode, rawURL)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", rawURL, err)
	}

	return data, nil
}

func isAbsoluteURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func parseAcceptanceCommand(command string) (inputDirective, string, error) {
	args, err := splitCommandLine(command)
	if err != nil {
		return inputDirective{}, "", err
	}

	var (
		directive inputDirective
		runSeen   bool
		service   string
		foundFlag bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if runSeen && service == "" && !strings.HasPrefix(arg, "-") {
			service = arg
			continue
		}

		switch arg {
		case "run":
			runSeen = true
		case "-f", "--file-input":
			if i+1 >= len(args) {
				return inputDirective{}, "", fmt.Errorf("flag %s missing value", arg)
			}
			directive = inputDirective{Mode: inputModeFile, Value: args[i+1]}
			foundFlag = true
			i++
		case "-i", "--text-input":
			if i+1 >= len(args) {
				return inputDirective{}, "", fmt.Errorf("flag %s missing value", arg)
			}
			directive = inputDirective{Mode: inputModeText, Value: args[i+1]}
			foundFlag = true
			i++
		}
	}

	if !foundFlag {
		return inputDirective{}, service, errCommandMissingInput
	}

	return directive, service, nil
}

func splitCommandLine(command string) ([]string, error) {
	var args []string
	var current bytes.Buffer
	var quote rune
	var escaping bool

	for _, r := range command {
		switch {
		case escaping:
			current.WriteRune(r)
			escaping = false
		case r == '\\':
			escaping = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			if current.Len() == 0 {
				continue
			}
		case isWhitespace(r):
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if escaping {
		return nil, errors.New("unterminated escape sequence in command")
	}

	if quote != 0 {
		return nil, errors.New("unterminated quoted string in command")
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args, nil
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func invokeServiceWithContent(clusterCfg *cluster.Cluster, serviceName string, payload []byte) ([]byte, error) {
	reader, writer := io.Pipe()
	go func() {
		encoder := base64.NewEncoder(base64.StdEncoding, writer)
		_, err := encoder.Write(payload)
		encoder.Close()
		if err != nil {
			writer.CloseWithError(err)
		} else {
			writer.Close()
		}
	}()

	response, err := service.RunService(clusterCfg, serviceName, "", "", reader)
	if err != nil {
		return nil, err
	}
	defer response.Close()

	raw, err := io.ReadAll(response)
	if err != nil {
		return nil, fmt.Errorf("reading service response: %w", err)
	}

	trimmed := bytes.TrimSpace(raw)
	decoded, decodeErr := base64.StdEncoding.DecodeString(string(trimmed))
	if decodeErr == nil {
		return decoded, nil
	}
	// Fallback to raw response when it is not base64 encoded.
	return raw, nil
}

func previewOutput(output string) string {
	if len(output) <= maxOutputPreview {
		return output
	}
	return output[:maxOutputPreview] + "..."
}
