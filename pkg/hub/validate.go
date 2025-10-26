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
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v3/pkg/types"
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

type stepCommandKind int

const (
	stepCommandUnknown stepCommandKind = iota
	stepCommandRun
	stepCommandPutFile
	stepCommandGetFile
	stepCommandWait
)

type parsedCommand struct {
	Kind            stepCommandKind
	ServiceName     string
	RunDirective    inputDirective
	Provider        string
	LocalPath       string
	RemotePath      string
	RemoteProvided  bool
	LocalProvided   bool
	LatestRequested bool
	LatestValue     string
	NoProgress      bool
	WaitDuration    time.Duration
}

// ValidateService downloads the RO-Crate metadata for the provided slug, runs its acceptance tests against the cluster and returns the aggregated results.
func (c *Client) ValidateService(ctx context.Context, slug string, clusterCfg *cluster.Cluster, serviceNameOverride string, localRoot string) ([]AcceptanceResult, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("service slug cannot be empty")
	}
	if clusterCfg == nil {
		return nil, errors.New("cluster configuration is required")
	}

	var (
		repoPath       string
		localCratePath string
		rawMetadata    []byte
		err            error
	)

	localRoot = strings.TrimSpace(localRoot)
	if localRoot != "" {
		rawMetadata, localCratePath, err = loadLocalMetadata(localRoot, slug)
		if err != nil {
			return nil, err
		}
	} else {
		repoPath = c.serviceRepoPath(slug)
		metadataPath := path.Join(repoPath, metadataFile)
		rawMetadata, err = c.getFile(ctx, metadataPath)
		if err != nil {
			return nil, err
		}
	}

	crate, err := ParseROCrate(rawMetadata)
	if err != nil {
		return nil, err
	}

	tests, err := crate.AcceptanceTests()
	if err != nil {
		return nil, err
	}

	serviceCache := make(map[string]*types.Service)
	results := make([]AcceptanceResult, 0, len(tests))
	for _, test := range tests {
		testName := strings.TrimSpace(test.Name)
		if testName == "" {
			testName = test.ID
		}
		c.logf("Running acceptance test: %s\n", testName)
		res := c.runAcceptanceTest(ctx, repoPath, slug, test, clusterCfg, serviceNameOverride, localCratePath, serviceCache)
		c.logAcceptanceResult(res)
		results = append(results, res)
	}

	return results, nil
}

func (c *Client) runAcceptanceTest(ctx context.Context, repoPath, slug string, test AcceptanceTest, clusterCfg *cluster.Cluster, serviceNameOverride string, localCratePath string, svcCache map[string]*types.Service) AcceptanceResult {
	result := AcceptanceResult{Test: test}

	steps := test.Steps
	if len(steps) == 0 {
		result.Err = fmt.Errorf("acceptance test %s does not define executable steps", test.ID)
		result.Passed = false
		return result
	}

	tempDir, err := os.MkdirTemp("", "oscar-cli-validate-*")
	if err != nil {
		result.Err = fmt.Errorf("creating temporary workspace for test %s: %w", test.ID, err)
		result.Passed = false
		return result
	}
	defer os.RemoveAll(tempDir)

	result.Passed = true
	supplyCache := buildTestSupplyMap(test)
	var lastOutput string

	for _, step := range steps {
		stepRes := c.executeAcceptanceStep(ctx, repoPath, slug, test, step, supplyCache, clusterCfg, serviceNameOverride, localCratePath, svcCache, tempDir)
		result.StepResults = append(result.StepResults, stepRes)

		if stepRes.Output != "" {
			lastOutput = stepRes.Output
		}

		if !stepRes.Passed {
			result.Passed = false
			if result.Err == nil && stepRes.Err != nil {
				result.Err = stepRes.Err
			}
			if result.Details == "" && stepRes.Details != "" {
				result.Details = stepRes.Details
			}
		}
	}

	result.Output = previewOutput(lastOutput)

	return result
}

func buildTestSupplyMap(test AcceptanceTest) map[string]TestInput {
	supply := make(map[string]TestInput, len(test.Inputs))
	for _, input := range test.Inputs {
		supply[input.ID] = input
	}
	return supply
}

func (c *Client) executeAcceptanceStep(ctx context.Context, repoPath, slug string, test AcceptanceTest, step AcceptanceStep, baseSupply map[string]TestInput, clusterCfg *cluster.Cluster, serviceNameOverride string, localCratePath string, svcCache map[string]*types.Service, tempDir string) AcceptanceStepResult {
	result := AcceptanceStepResult{Step: step}

	if strings.TrimSpace(step.Command) == "" {
		result.Err = fmt.Errorf("step %s does not define a command", step.ID)
		return result
	}

	parsed := step.ParsedCommand
	if parsed == nil {
		tmp, err := parseAcceptanceCommand(step.Command)
		if err != nil {
			result.Err = fmt.Errorf("parsing command for step %s: %w", step.ID, err)
			return result
		}
		parsed = &tmp
	}

	serviceName := parsed.ServiceName
	if strings.TrimSpace(serviceNameOverride) != "" {
		serviceName = serviceNameOverride
	}
	if strings.TrimSpace(serviceName) == "" {
		serviceName = slug
	}

	supply := mergeSupplyMaps(baseSupply, step.Inputs)

	switch parsed.Kind {
	case stepCommandRun:
		payload, err := resolveRunPayload(ctx, parsed.RunDirective, supply, c, repoPath, localCratePath)
		if err != nil {
			result.Err = err
			return result
		}

		responseBytes, err := invokeServiceWithContent(clusterCfg, serviceName, payload)
		if err != nil {
			result.Err = err
			return result
		}

		output := string(responseBytes)
		result.Passed, result.Details = evaluateExpectation(step.ExpectedSubstring, output)
		result.Output = previewOutput(output)
	case stepCommandPutFile:
		svc, err := getServiceDefinition(clusterCfg, serviceName, svcCache)
		if err != nil {
			result.Err = err
			return result
		}

		provider := parsed.Provider
		if strings.TrimSpace(provider) == "" && len(storage.DefaultStorageProvider) > 0 {
			provider = storage.DefaultStorageProvider[0]
		}

		content, err := resolveUploadContent(ctx, parsed.LocalPath, supply, c, repoPath, localCratePath)
		if err != nil {
			result.Err = err
			return result
		}

		targetPath, err := writeTempContent(tempDir, parsed.LocalPath, content)
		if err != nil {
			result.Err = err
			return result
		}

		remotePath := parsed.RemotePath
		if !parsed.RemoteProvided {
			remotePath, err = storage.DefaultRemotePath(svc, provider, parsed.LocalPath)
			if err != nil {
				result.Err = err
				return result
			}
		}

		err = storage.PutFileWithService(clusterCfg, svc, provider, targetPath, remotePath, &storage.TransferOption{ShowProgress: false})
		if err != nil {
			result.Err = err
			return result
		}

		result.Output = fmt.Sprintf("%s -> %s", parsed.LocalPath, remotePath)
		result.Passed = true
	case stepCommandGetFile:
		svc, err := getServiceDefinition(clusterCfg, serviceName, svcCache)
		if err != nil {
			result.Err = err
			return result
		}

		provider := parsed.Provider
		if strings.TrimSpace(provider) == "" {
			provider, err = storage.DefaultOutputProvider(svc)
			if err != nil {
				result.Err = err
				return result
			}
		}

		scopePath := parsed.RemotePath
		if !parsed.RemoteProvided {
			if !parsed.LatestRequested {
				result.Err = fmt.Errorf("step %s requires a remote path or --download-latest-into flag", step.ID)
				return result
			}
			scopePath, err = storage.DefaultOutputPath(svc, provider)
			if err != nil {
				result.Err = err
				return result
			}
		}

		remotePath := parsed.RemotePath
		if parsed.LatestRequested {
			basePath := scopePath
			if parsed.RemoteProvided {
				basePath = parsed.RemotePath
			}
			remotePath, err = storage.ResolveLatestRemotePath(clusterCfg, svc, provider, basePath)
			if err != nil {
				result.Err = err
				return result
			}

			if parsed.LatestValue != "" && parsed.LocalProvided {
				result.Err = fmt.Errorf("step %s: --download-latest-into already defines a destination path", step.ID)
				return result
			}

			if !parsed.LocalProvided {
				baseName := filepath.Base(remotePath)
				if baseName == "" || baseName == "." || baseName == "/" {
					result.Err = fmt.Errorf("step %s: unable to infer local name from remote path %q", step.ID, remotePath)
					return result
				}
				parsed.LocalPath = resolveLatestDestination(parsed.LatestValue, baseName)
				parsed.LocalProvided = true
			}
		}

		if !parsed.LocalProvided {
			result.Err = fmt.Errorf("step %s requires a local destination path", step.ID)
			return result
		}

		targetPath, err := writeTempContent(tempDir, parsed.LocalPath, nil)
		if err != nil {
			result.Err = err
			return result
		}

		err = storage.GetFileWithService(clusterCfg, svc, provider, remotePath, targetPath, &storage.TransferOption{ShowProgress: false})
		if err != nil {
			result.Err = err
			return result
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Err = err
			return result
		}

		if len(step.ExpectedMedia) > 0 {
			detected := http.DetectContentType(data)
			if !mediaTypeMatches(detected, step.ExpectedMedia) {
				result.Passed = false
				result.Details = fmt.Sprintf("expected media type %s, got %s", strings.Join(step.ExpectedMedia, ", "), detected)
				result.Output = fmt.Sprintf("Detected media type: %s", detected)
				return result
			}
			result.Passed = true
			result.Output = fmt.Sprintf("Detected media type: %s", detected)
		} else {
			output := string(data)
			result.Passed, result.Details = evaluateExpectation(step.ExpectedSubstring, output)
			result.Output = previewOutput(output)
		}
	case stepCommandWait:
		if parsed.WaitDuration <= 0 {
			result.Passed = true
			result.Output = "Wait skipped (duration 0)"
			return result
		}

		timer := time.NewTimer(parsed.WaitDuration)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			result.Err = ctx.Err()
			return result
		case <-timer.C:
			result.Passed = true
			result.Output = fmt.Sprintf("Waited %s", parsed.WaitDuration)
		}
	default:
		result.Err = fmt.Errorf("unsupported command for step %s: %s", step.ID, step.Command)
		return result
	}

	return result
}

func mergeSupplyMaps(base map[string]TestInput, stepInputs []TestInput) map[string]TestInput {
	supply := make(map[string]TestInput, len(base)+len(stepInputs))
	for id, input := range base {
		supply[id] = input
	}
	for _, input := range stepInputs {
		supply[input.ID] = input
	}
	return supply
}

func resolveRunPayload(ctx context.Context, directive inputDirective, supply map[string]TestInput, client *Client, repoPath, localCratePath string) ([]byte, error) {
	switch directive.Mode {
	case inputModeFile:
		input, ok := supply[directive.Value]
		if !ok {
			return nil, fmt.Errorf("input %q referenced in command not found in RO-Crate supply list", directive.Value)
		}
		return fetchSupplyContent(ctx, client, repoPath, localCratePath, input)
	case inputModeText:
		if input, ok := supply[directive.Value]; ok {
			return fetchSupplyContent(ctx, client, repoPath, localCratePath, input)
		}
		return []byte(directive.Value), nil
	default:
		return nil, errCommandMissingInput
	}
}

func resolveUploadContent(ctx context.Context, localPath string, supply map[string]TestInput, client *Client, repoPath, localCratePath string) ([]byte, error) {
	if input, ok := supply[localPath]; ok {
		return fetchSupplyContent(ctx, client, repoPath, localCratePath, input)
	}

	// Attempt to match on the basename when the command uses a relative path.
	base := filepath.Base(localPath)
	for _, input := range supply {
		if filepath.Base(input.ID) == base {
			return fetchSupplyContent(ctx, client, repoPath, localCratePath, input)
		}
	}

	fallback := TestInput{ID: localPath}
	return fetchSupplyContent(ctx, client, repoPath, localCratePath, fallback)
}

func ensureSafeRelativePath(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("empty path")
	}

	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path not allowed: %s", relative)
	}
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("path %q escapes temporary workspace", relative)
	}
	return clean, nil
}

func writeTempContent(baseDir, target string, data []byte) (string, error) {
	clean, err := ensureSafeRelativePath(target)
	if err != nil {
		// Fall back to using only the base name
		clean, err = ensureSafeRelativePath(filepath.Base(target))
		if err != nil {
			return "", err
		}
	}

	dest := filepath.Join(baseDir, clean)

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory for %s: %w", dest, err)
	}

	if data == nil {
		// Ensure the file exists and is empty
		file, err := os.Create(dest)
		if err != nil {
			return "", fmt.Errorf("creating file %s: %w", dest, err)
		}
		file.Close()
		return dest, nil
	}

	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("writing temporary file %s: %w", dest, err)
	}

	return dest, nil
}

func getServiceDefinition(clusterCfg *cluster.Cluster, serviceName string, cache map[string]*types.Service) (*types.Service, error) {
	if svc, ok := cache[serviceName]; ok && svc != nil {
		return svc, nil
	}

	svc, err := service.GetService(clusterCfg, serviceName)
	if err != nil {
		return nil, err
	}
	cache[serviceName] = svc
	return svc, nil
}

func evaluateExpectation(expected, output string) (bool, string) {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true, ""
	}
	if strings.Contains(output, expected) {
		return true, ""
	}
	return false, fmt.Sprintf("expected substring %q not found", expected)
}

func mediaTypeMatches(detected string, expected []string) bool {
	detected = normalizeMediaType(detected)
	if detected == "" {
		return false
	}
	for _, exp := range expected {
		if normalizeMediaType(exp) == detected {
			return true
		}
	}
	return false
}

func normalizeMediaType(mt string) string {
	mt = strings.ToLower(strings.TrimSpace(mt))
	if mt == "" {
		return ""
	}
	if idx := strings.Index(mt, ";"); idx >= 0 {
		mt = mt[:idx]
	}
	return strings.TrimSpace(mt)
}

func resolveLatestDestination(flagValue, baseName string) string {
	flagValue = strings.TrimSpace(flagValue)
	if flagValue == "" {
		return filepath.Join(".", baseName)
	}

	clean := filepath.Clean(flagValue)
	if len(flagValue) > 0 && os.IsPathSeparator(flagValue[len(flagValue)-1]) {
		return filepath.Join(clean, baseName)
	}

	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		return filepath.Join(clean, baseName)
	}

	if filepath.Ext(clean) != "" {
		return clean
	}

	return filepath.Join(clean, baseName)
}

func fetchSupplyContent(ctx context.Context, client *Client, repoPath string, localCratePath string, input TestInput) ([]byte, error) {
	candidates := make([]string, 0, 2)
	if url := strings.TrimSpace(input.URL); url != "" {
		candidates = append(candidates, url)
	}
	if id := strings.TrimSpace(input.ID); id != "" {
		candidates = append(candidates, id)
	}

	var lastErr error
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		if isAbsoluteURL(candidate) {
			data, err := downloadExternalResource(ctx, candidate)
			if err == nil {
				return data, nil
			}
			lastErr = err
			continue
		}

		if localCratePath != "" {
			data, err := readFromLocal(localCratePath, candidate)
			if err == nil {
				return data, nil
			}
			if errors.Is(err, errEscapesServiceDirectory) {
				return nil, err
			}
			lastErr = err
		}

		if repoPath != "" && client != nil {
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
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("unable to resolve input %q", input.ID)
}

var errEscapesServiceDirectory = errors.New("path escapes service directory")

func loadLocalMetadata(localRoot, slug string) ([]byte, string, error) {
	localRoot = filepath.Clean(localRoot)

	info, err := os.Stat(localRoot)
	if err != nil {
		return nil, "", fmt.Errorf("checking local path %s: %w", localRoot, err)
	}

	candidates := make([]string, 0, 2)
	if info.IsDir() {
		candidates = append(candidates, localRoot, filepath.Join(localRoot, slug))
	} else {
		// Allow pointing directly to the metadata file.
		candidates = append(candidates, filepath.Dir(localRoot))
		if strings.EqualFold(filepath.Base(localRoot), metadataFile) {
			data, err := os.ReadFile(localRoot)
			if err != nil {
				return nil, "", fmt.Errorf("reading metadata file %s: %w", localRoot, err)
			}
			return data, filepath.Dir(localRoot), nil
		}
	}

	for _, dir := range candidates {
		dir = filepath.Clean(dir)
		metadataPath := filepath.Join(dir, metadataFile)
		data, err := os.ReadFile(metadataPath)
		if err == nil {
			return data, dir, nil
		}
	}

	return nil, "", fmt.Errorf("ro-crate metadata not found for %s under %s", slug, localRoot)
}

func readFromRepository(ctx context.Context, client *Client, repoPath, relative string) ([]byte, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repository path not available")
	}
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

func readFromLocal(baseDir, relative string) ([]byte, error) {
	clean := filepath.Clean(strings.TrimSpace(relative))
	if clean == "." || clean == "" {
		return nil, fmt.Errorf("invalid input path %q", relative)
	}
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("%w: %s", errEscapesServiceDirectory, relative)
	}

	fullPath := filepath.Join(baseDir, clean)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	return data, nil
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

func parseAcceptanceCommand(command string) (parsedCommand, error) {
	args, err := splitCommandLine(command)
	if err != nil {
		return parsedCommand{}, err
	}

	if len(args) == 0 {
		return parsedCommand{}, fmt.Errorf("command is empty")
	}

	if len(args) > 0 && (args[0] == "oscar-cli" || args[0] == "ocli-dev") {
		args = args[1:]
	}

	if len(args) == 0 {
		return parsedCommand{}, fmt.Errorf("command missing service subcommand")
	}

	if args[0] != "service" {
		return parsedCommand{}, fmt.Errorf("unsupported command prefix %q", args[0])
	}

	if len(args) < 2 {
		return parsedCommand{}, fmt.Errorf("service command missing subcommand")
	}

	action := args[1]
	rest := args[2:]

	switch action {
	case "run":
		return parseServiceRun(rest)
	case "put-file":
		return parseServicePutFile(rest)
	case "get-file":
		return parseServiceGetFile(rest)
	default:
		return parsedCommand{}, fmt.Errorf("unsupported service subcommand %q", action)
	}
}

func parseServiceRun(args []string) (parsedCommand, error) {
	parsed := parsedCommand{Kind: stepCommandRun}
	var (
		directive inputDirective
		foundFlag bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-f", "--file-input":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			directive = inputDirective{Mode: inputModeFile, Value: args[i+1]}
			foundFlag = true
			i++
		case "-i", "--text-input":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			directive = inputDirective{Mode: inputModeText, Value: args[i+1]}
			foundFlag = true
			i++
		default:
			if parsed.ServiceName == "" && !strings.HasPrefix(arg, "-") {
				parsed.ServiceName = arg
			}
		}
	}

	if !foundFlag {
		return parsedCommand{}, errCommandMissingInput
	}

	parsed.RunDirective = directive
	return parsed, nil
}

func parseServicePutFile(args []string) (parsedCommand, error) {
	parsed := parsedCommand{Kind: stepCommandPutFile}
	if len(args) == 0 {
		return parsedCommand{}, fmt.Errorf("service put-file requires SERVICE_NAME argument")
	}

	parsed.ServiceName = args[0]
	if parsed.ServiceName == "" {
		return parsedCommand{}, fmt.Errorf("service name cannot be empty")
	}

	if len(args) == 1 {
		return parsedCommand{}, fmt.Errorf("service put-file requires LOCAL_FILE argument")
	}

	provider, localFile, remoteFile, remoteProvided, err := parsePutFileCommandArgs(args[1:])
	if err != nil {
		return parsedCommand{}, err
	}

	parsed.Provider = provider
	parsed.LocalPath = localFile
	parsed.RemotePath = remoteFile
	parsed.RemoteProvided = remoteProvided

	return parsed, nil
}

func parseServiceGetFile(args []string) (parsedCommand, error) {
	parsed := parsedCommand{Kind: stepCommandGetFile}
	if len(args) == 0 {
		return parsedCommand{}, fmt.Errorf("service get-file requires SERVICE_NAME argument")
	}

	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--download-latest-into":
			parsed.LatestRequested = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				parsed.LatestValue = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--download-latest-into="):
			parsed.LatestRequested = true
			parsed.LatestValue = strings.TrimPrefix(arg, "--download-latest-into=")
		case arg == "--no-progress":
			parsed.NoProgress = true
		case strings.HasPrefix(arg, "--"):
			return parsedCommand{}, fmt.Errorf("unsupported flag %q in get-file command", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return parsedCommand{}, fmt.Errorf("service name cannot be empty")
	}

	parsed.ServiceName = positional[0]
	provider, remotePath, localPath, remoteProvided, localProvided, err := parseGetFileCommandArgs(positional[1:], parsed.LatestRequested)
	if err != nil {
		return parsedCommand{}, err
	}

	parsed.Provider = provider
	parsed.RemotePath = remotePath
	parsed.LocalPath = localPath
	parsed.RemoteProvided = remoteProvided
	parsed.LocalProvided = localProvided

	return parsed, nil
}

func parsePutFileCommandArgs(args []string) (provider, localFile, remoteFile string, remoteProvided bool, err error) {
	defaultProvider := defaultStorageProvider()

	switch len(args) {
	case 1:
		return defaultProvider, args[0], "", false, nil
	case 2:
		if looksLikeStorageProvider(args[0]) {
			return args[0], args[1], "", false, nil
		}
		return defaultProvider, args[0], args[1], true, nil
	case 3:
		if !looksLikeStorageProvider(args[0]) {
			return "", "", "", false, fmt.Errorf("invalid storage provider %q", args[0])
		}
		return args[0], args[1], args[2], true, nil
	default:
		return "", "", "", false, fmt.Errorf("invalid number of arguments for put-file command")
	}
}

func parseGetFileCommandArgs(args []string, allowRemoteOmit bool) (provider, remotePath, localPath string, remoteProvided, localProvided bool, err error) {
	switch len(args) {
	case 0:
		if !allowRemoteOmit {
			return "", "", "", false, false, fmt.Errorf("invalid number of arguments")
		}
		return "", "", "", false, false, nil
	case 1:
		if looksLikeStorageProvider(args[0]) {
			if !allowRemoteOmit {
				return "", "", "", false, false, fmt.Errorf("REMOTE_PATH argument is required")
			}
			return args[0], "", "", false, false, nil
		}
		remotePath = args[0]
		return "", remotePath, filepath.Base(remotePath), true, true, nil
	case 2:
		if looksLikeStorageProvider(args[0]) {
			remotePath = args[1]
			return args[0], remotePath, filepath.Base(remotePath), true, true, nil
		}
		return "", args[0], args[1], true, true, nil
	case 3:
		if !looksLikeStorageProvider(args[0]) {
			return "", "", "", false, false, fmt.Errorf("invalid storage provider %q", args[0])
		}
		return args[0], args[1], args[2], true, true, nil
	default:
		return "", "", "", false, false, fmt.Errorf("invalid number of arguments")
	}
}

func looksLikeStorageProvider(value string) bool {
	parts := strings.SplitN(value, types.ProviderSeparator, 2)
	if len(parts) == 1 && slices.Contains(storage.DefaultStorageProvider, parts[0]) {
		return true
	}
	if len(parts) != 2 {
		return false
	}
	switch parts[0] {
	case types.MinIOName, types.S3Name, types.OnedataName, types.WebDavName:
		return true
	default:
		return false
	}
}

func defaultStorageProvider() string {
	if len(storage.DefaultStorageProvider) > 0 {
		return storage.DefaultStorageProvider[0]
	}
	return "minio.default"
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
