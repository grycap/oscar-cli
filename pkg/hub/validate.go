package hub

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v4/pkg/types"
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
	stepCommandHTTP
)

type parsedCommand struct {
	Kind               stepCommandKind
	ServiceName        string
	RunDirective       inputDirective
	Provider           string
	LocalPath          string
	RemotePath         string
	RemoteProvided     bool
	LocalProvided      bool
	LatestRequested    bool
	LatestValue        string
	NoProgress         bool
	WaitDuration       time.Duration
	HTTPMethod         string
	HTTPPath           string
	HTTPAccept         string
	HTTPFormField      string
	HTTPUseServiceAuth bool
	HTTPExpectStatus   int
}

// AcceptanceCommandSet stores runnable shell commands for a single acceptance test.
type AcceptanceCommandSet struct {
	Test     AcceptanceTest
	Commands []string
}

// ValidateService downloads the RO-Crate metadata for the provided slug, runs its acceptance tests against the cluster and returns the aggregated results.
func (c *Client) ValidateService(ctx context.Context, slug string, clusterCfg *cluster.Cluster, serviceNameOverride string, localRoot string) ([]AcceptanceResult, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("service slug cannot be empty")
	}
	if clusterCfg == nil {
		return nil, errors.New("cluster configuration is required")
	}

	repoPath, localCratePath, tests, err := c.loadAcceptanceTests(ctx, slug, localRoot)
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

// AcceptanceCommands renders the acceptance tests for the provided slug as runnable shell commands.
func (c *Client) AcceptanceCommands(ctx context.Context, slug string, serviceNameOverride string, localRoot string) ([]AcceptanceCommandSet, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("service slug cannot be empty")
	}

	_, localCratePath, tests, err := c.loadAcceptanceTests(ctx, slug, localRoot)
	if err != nil {
		return nil, err
	}

	sets := make([]AcceptanceCommandSet, 0, len(tests))
	for _, test := range tests {
		set := AcceptanceCommandSet{
			Test:     test,
			Commands: renderAcceptanceCommands(test, slug, serviceNameOverride, localCratePath),
		}
		sets = append(sets, set)
	}

	return sets, nil
}

func (c *Client) loadAcceptanceTests(ctx context.Context, slug string, localRoot string) (string, string, []AcceptanceTest, error) {

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
			return "", "", nil, err
		}
	} else {
		repoPath = c.serviceRepoPath(slug)
		metadataPath := path.Join(repoPath, metadataFile)
		rawMetadata, err = c.getFile(ctx, metadataPath)
		if err != nil {
			return "", "", nil, err
		}
	}

	crate, err := ParseROCrate(rawMetadata)
	if err != nil {
		return "", "", nil, err
	}

	tests, err := crate.AcceptanceTests()
	if err != nil {
		return "", "", nil, err
	}

	return repoPath, localCratePath, tests, nil
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

func renderAcceptanceCommands(test AcceptanceTest, slug string, serviceNameOverride string, localCratePath string) []string {
	if len(test.Steps) == 0 {
		return nil
	}

	supply := buildTestSupplyMap(test)
	commands := make([]string, 0, len(test.Steps))
	for _, step := range test.Steps {
		parsed := step.ParsedCommand
		if parsed == nil && strings.TrimSpace(step.Command) != "" {
			tmp, err := parseAcceptanceCommand(step.Command)
			if err == nil {
				parsed = &tmp
			}
		}
		if parsed == nil {
			continue
		}

		serviceName := strings.TrimSpace(parsed.ServiceName)
		if strings.TrimSpace(serviceNameOverride) != "" {
			serviceName = strings.TrimSpace(serviceNameOverride)
		}
		if serviceName == "" {
			serviceName = slug
		}

		stepSupply := mergeSupplyMaps(supply, step.Inputs)
		if rendered := renderParsedCommand(*parsed, step, serviceName, stepSupply, localCratePath); rendered != "" {
			commands = append(commands, rendered)
		}
	}
	return commands
}

func renderParsedCommand(cmd parsedCommand, step AcceptanceStep, serviceName string, supply map[string]TestInput, localCratePath string) string {
	switch cmd.Kind {
	case stepCommandRun:
		args := []string{"oscar-cli", "service", "run", serviceName}
		switch cmd.RunDirective.Mode {
		case inputModeFile:
			args = append(args, "--file-input", resolveInputPath(cmd.RunDirective.Value, supply, localCratePath))
		case inputModeText:
			args = append(args, "--text-input", cmd.RunDirective.Value)
		default:
			return ""
		}
		return shellJoin(args)
	case stepCommandPutFile:
		args := []string{"oscar-cli", "service", "put-file", serviceName}
		if strings.TrimSpace(cmd.Provider) != "" {
			args = append(args, cmd.Provider)
		}
		args = append(args, resolveInputPath(cmd.LocalPath, supply, localCratePath))
		if strings.TrimSpace(cmd.RemotePath) != "" {
			args = append(args, cmd.RemotePath)
		}
		return shellJoin(args)
	case stepCommandGetFile:
		args := []string{"oscar-cli", "service", "get-file", serviceName}
		if strings.TrimSpace(cmd.Provider) != "" {
			args = append(args, cmd.Provider)
		}
		if strings.TrimSpace(cmd.RemotePath) != "" {
			args = append(args, cmd.RemotePath)
		}
		if cmd.LatestRequested {
			destination := cmd.LatestValue
			if strings.TrimSpace(destination) == "" {
				destination = "./downloads"
			}
			args = append(args, "--download-latest-into", destination)
		} else if strings.TrimSpace(cmd.LocalPath) != "" {
			args = append(args, cmd.LocalPath)
		}
		return shellJoin(args)
	case stepCommandWait:
		if cmd.WaitDuration <= 0 {
			return ""
		}
		return shellJoin([]string{"sleep", strconv.Itoa(int(cmd.WaitDuration.Round(time.Second).Seconds()))})
	case stepCommandHTTP:
		return renderHTTPCurlCommand(cmd, step, serviceName, supply, localCratePath)
	default:
		return ""
	}
}

func renderHTTPCurlCommand(cmd parsedCommand, step AcceptanceStep, serviceName string, supply map[string]TestInput, localCratePath string) string {
	args := []string{"curl", "-sS"}
	method := strings.ToUpper(strings.TrimSpace(cmd.HTTPMethod))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		args = append(args, "-X", method)
	}
	if cmd.HTTPUseServiceAuth {
		args = append(args, "-u", fmt.Sprintf("%s:${SERVICE_TOKEN}", serviceName))
	}
	if strings.TrimSpace(cmd.HTTPAccept) != "" {
		args = append(args, "-H", "Accept: "+cmd.HTTPAccept)
	}
	if len(step.Inputs) > 0 {
		fieldName := strings.TrimSpace(cmd.HTTPFormField)
		if fieldName == "" {
			fieldName = "file"
		}
		for _, input := range step.Inputs {
			path := resolveInputPath(input.ID, supply, localCratePath)
			formValue := fmt.Sprintf("%s=@%s", fieldName, path)
			if strings.TrimSpace(input.EncodingFormat) != "" {
				formValue += ";type=" + input.EncodingFormat
			}
			args = append(args, "-F", formValue)
		}
	}
	if outputPath := suggestedOutputPath(step); outputPath != "" {
		args = append(args, "--output", outputPath)
	}
	requestPath := strings.TrimSpace(cmd.HTTPPath)
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	args = append(args, "${OSCAR_ENDPOINT%/}/system/services/"+serviceName+"/exposed"+requestPath)
	return shellJoin(args)
}

func suggestedOutputPath(step AcceptanceStep) string {
	for _, mt := range step.ExpectedMedia {
		switch normalizeMediaType(mt) {
		case "application/zip":
			return "./acceptance-output.zip"
		case "image/png":
			return "./acceptance-output.png"
		case "image/jpeg":
			return "./acceptance-output.jpg"
		}
	}
	return ""
}

func resolveInputPath(value string, supply map[string]TestInput, localCratePath string) string {
	if input, ok := supply[value]; ok {
		if path := localInputPath(input, localCratePath); path != "" {
			return path
		}
		if strings.TrimSpace(input.URL) != "" {
			return input.URL
		}
		if strings.TrimSpace(input.ID) != "" {
			return "./" + filepath.Base(input.ID)
		}
	}
	if path := localInputPath(TestInput{ID: value}, localCratePath); path != "" {
		return path
	}
	if strings.TrimSpace(value) == "" {
		return "./input"
	}
	return "./" + filepath.Base(value)
}

func localInputPath(input TestInput, localCratePath string) string {
	if strings.TrimSpace(localCratePath) == "" {
		return ""
	}
	candidates := []string{input.ID, input.URL}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || isAbsoluteURL(candidate) {
			continue
		}
		clean := filepath.Clean(candidate)
		if clean == "." || strings.HasPrefix(clean, "..") {
			continue
		}
		full := filepath.Join(localCratePath, clean)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(full); err == nil {
				return abs
			}
			return full
		}
	}
	return ""
}

func shellJoin(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.Contains(value, "${") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	if strings.ContainsAny(value, " \t\n'\"$;&|<>*?()[]{}!") {
		return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
	}
	return value
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
	case stepCommandHTTP:
		svc, err := getServiceDefinition(clusterCfg, serviceName, svcCache)
		if err != nil {
			result.Err = err
			return result
		}

		payloads, err := resolveHTTPPayloads(ctx, supply, step.Inputs, c, repoPath, localCratePath)
		if err != nil {
			result.Err = err
			return result
		}

		responseData, responseMedia, statusCode, err := invokeExposedHTTP(clusterCfg, svc, serviceName, *parsed, payloads)
		if err != nil {
			result.Err = err
			return result
		}

		expectedStatus := parsed.HTTPExpectStatus
		if expectedStatus == 0 {
			expectedStatus = http.StatusOK
		}
		if statusCode != expectedStatus {
			result.Passed = false
			result.Details = fmt.Sprintf("expected HTTP status %d, got %d", expectedStatus, statusCode)
			result.Output = fmt.Sprintf("HTTP %d", statusCode)
			return result
		}

		if len(step.ExpectedMedia) > 0 {
			detected := firstMatchingMediaType(responseMedia, http.DetectContentType(responseData))
			if !mediaTypeMatches(detected, step.ExpectedMedia) {
				result.Passed = false
				result.Details = fmt.Sprintf("expected media type %s, got %s", strings.Join(step.ExpectedMedia, ", "), detected)
				result.Output = fmt.Sprintf("HTTP %d, media type: %s", statusCode, detected)
				return result
			}

			if normalizeMediaType(detected) == "application/zip" {
				if err := validateZIPPayload(responseData); err != nil {
					result.Passed = false
					result.Details = err.Error()
					result.Output = fmt.Sprintf("HTTP %d, media type: %s", statusCode, detected)
					return result
				}
			}

			result.Passed = true
			result.Output = fmt.Sprintf("HTTP %d, media type: %s, bytes: %d", statusCode, detected, len(responseData))
			return result
		}

		output := string(responseData)
		result.Passed, result.Details = evaluateExpectation(step.ExpectedSubstring, output)
		result.Output = previewOutput(output)
	default:
		result.Err = fmt.Errorf("unsupported command for step %s: %s", step.ID, step.Command)
		return result
	}

	return result
}

type httpPayload struct {
	Name        string
	Content     []byte
	ContentType string
}

func resolveHTTPPayloads(ctx context.Context, supply map[string]TestInput, stepInputs []TestInput, client *Client, repoPath, localCratePath string) ([]httpPayload, error) {
	if len(stepInputs) == 0 {
		return nil, errCommandMissingInput
	}

	payloads := make([]httpPayload, 0, len(stepInputs))
	for _, input := range stepInputs {
		content, err := fetchSupplyContent(ctx, client, repoPath, localCratePath, input)
		if err != nil {
			return nil, err
		}

		name := strings.TrimSpace(input.ID)
		if name == "" {
			name = "input"
		}
		payloads = append(payloads, httpPayload{
			Name:        filepath.Base(name),
			Content:     content,
			ContentType: strings.TrimSpace(input.EncodingFormat),
		})
	}
	return payloads, nil
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
	case "http":
		return parseServiceHTTP(rest)
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

func parseServiceHTTP(args []string) (parsedCommand, error) {
	parsed := parsedCommand{
		Kind:               stepCommandHTTP,
		HTTPMethod:         http.MethodGet,
		HTTPUseServiceAuth: true,
		HTTPExpectStatus:   http.StatusOK,
	}
	if len(args) == 0 {
		return parsedCommand{}, fmt.Errorf("service http requires SERVICE_NAME argument")
	}

	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--method":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			parsed.HTTPMethod = strings.ToUpper(strings.TrimSpace(args[i+1]))
			i++
		case "--path":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			parsed.HTTPPath = strings.TrimSpace(args[i+1])
			i++
		case "--accept":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			parsed.HTTPAccept = strings.TrimSpace(args[i+1])
			i++
		case "--form-field":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			parsed.HTTPFormField = strings.TrimSpace(args[i+1])
			i++
		case "--no-service-auth":
			parsed.HTTPUseServiceAuth = false
		case "--status":
			if i+1 >= len(args) {
				return parsedCommand{}, fmt.Errorf("flag %s missing value", arg)
			}
			status, err := parsePositiveInt(args[i+1])
			if err != nil {
				return parsedCommand{}, fmt.Errorf("invalid status code %q", args[i+1])
			}
			parsed.HTTPExpectStatus = status
			i++
		default:
			if strings.HasPrefix(arg, "--") {
				return parsedCommand{}, fmt.Errorf("unsupported flag %q in http command", arg)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return parsedCommand{}, fmt.Errorf("service name cannot be empty")
	}
	parsed.ServiceName = positional[0]
	if parsed.HTTPPath == "" {
		return parsedCommand{}, fmt.Errorf("service http requires --path")
	}
	if parsed.HTTPMethod == "" {
		parsed.HTTPMethod = http.MethodGet
	}
	return parsed, nil
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return value, nil
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

func invokeExposedHTTP(clusterCfg *cluster.Cluster, svc *types.Service, serviceName string, cmd parsedCommand, payloads []httpPayload) ([]byte, string, int, error) {
	if clusterCfg == nil {
		return nil, "", 0, errors.New("cluster configuration is required")
	}
	if svc == nil {
		return nil, "", 0, errors.New("service definition is required")
	}

	baseURL, err := url.Parse(clusterCfg.Endpoint)
	if err != nil {
		return nil, "", 0, cluster.ErrParsingEndpoint
	}

	requestPath := strings.TrimSpace(cmd.HTTPPath)
	if requestPath == "" {
		return nil, "", 0, errors.New("http path cannot be empty")
	}
	hasTrailingSlash := strings.HasSuffix(requestPath, "/")
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	baseURL.Path = path.Join(baseURL.Path, "/system/services", serviceName, "exposed")
	baseURL.Path = path.Join(baseURL.Path, requestPath)
	if hasTrailingSlash && !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}

	var body io.Reader
	contentType := ""
	method := strings.ToUpper(strings.TrimSpace(cmd.HTTPMethod))
	if method == "" {
		method = http.MethodGet
	}

	if len(payloads) > 0 {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		fieldName := strings.TrimSpace(cmd.HTTPFormField)
		if fieldName == "" {
			fieldName = "file"
		}
		for _, payload := range payloads {
			header := textproto.MIMEHeader{}
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(fieldName), escapeQuotes(payload.Name)))
			partType := strings.TrimSpace(payload.ContentType)
			if partType == "" {
				partType = mime.TypeByExtension(filepath.Ext(payload.Name))
			}
			if partType == "" {
				partType = "application/octet-stream"
			}
			header.Set("Content-Type", partType)
			part, err := writer.CreatePart(header)
			if err != nil {
				return nil, "", 0, fmt.Errorf("creating multipart field: %w", err)
			}
			if _, err := part.Write(payload.Content); err != nil {
				return nil, "", 0, fmt.Errorf("writing multipart content: %w", err)
			}
		}
		if err := writer.Close(); err != nil {
			return nil, "", 0, fmt.Errorf("closing multipart body: %w", err)
		}
		body = &buffer
		contentType = writer.FormDataContentType()
	}

	req, err := http.NewRequest(method, baseURL.String(), body)
	if err != nil {
		return nil, "", 0, cluster.ErrMakingRequest
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept := strings.TrimSpace(cmd.HTTPAccept); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if cmd.HTTPUseServiceAuth && svc.Expose.SetAuth {
		req.SetBasicAuth(serviceName, svc.Token)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !clusterCfg.SSLVerify},
		},
		Timeout: 30 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, "", 0, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", 0, fmt.Errorf("reading http response: %w", err)
	}

	return data, res.Header.Get("Content-Type"), res.StatusCode, nil
}

func firstMatchingMediaType(values ...string) string {
	for _, value := range values {
		if normalized := normalizeMediaType(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func validateZIPPayload(data []byte) error {
	readerAt := bytes.NewReader(data)
	archive, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return fmt.Errorf("response is not a valid zip archive: %w", err)
	}
	if len(archive.File) == 0 {
		return errors.New("zip archive is empty")
	}
	return nil
}

func escapeQuotes(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
	return replacer.Replace(value)
}

func previewOutput(output string) string {
	if len(output) <= maxOutputPreview {
		return output
	}
	return output[:maxOutputPreview] + "..."
}
