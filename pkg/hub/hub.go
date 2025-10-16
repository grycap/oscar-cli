package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/service"
)

const (
	defaultOwner   = "grycap"
	defaultRepo    = "oscar-hub"
	defaultRef     = "main"
	defaultPath    = ""
	defaultBaseAPI = "https://api.github.com"
	userAgent      = "oscar-cli"

	metadataFile = "ro-crate-metadata.json"
)

var (
	// ErrMetadataNotFound is returned when a service directory does not contain a RO-Crate metadata file.
	ErrMetadataNotFound = errors.New("metadata file not found")
	// ErrNotFound indicates a GitHub resource was not found.
	ErrNotFound = errors.New("resource not found")
)

// Client retrieves curated services from OSCAR Hub repositories.
type Client struct {
	owner      string
	repo       string
	rootPath   string
	ref        string
	baseAPI    string
	httpClient *http.Client
}

// Option mutates the client configuration.
type Option func(*Client)

// WithOwner overrides the GitHub owner.
func WithOwner(owner string) Option {
	return func(c *Client) {
		if owner != "" {
			c.owner = owner
		}
	}
}

// WithRepo overrides the GitHub repository.
func WithRepo(repo string) Option {
	return func(c *Client) {
		if repo != "" {
			c.repo = repo
		}
	}
}

// WithRootPath selects the subdirectory that holds curated services.
func WithRootPath(root string) Option {
	return func(c *Client) {
		trimmed := strings.Trim(root, "/")
		if trimmed == "." {
			trimmed = ""
		}
		c.rootPath = trimmed
	}
}

// WithRef selects a branch, tag, or commit.
func WithRef(ref string) Option {
	return func(c *Client) {
		if ref != "" {
			c.ref = ref
		}
	}
}

// WithBaseAPI sets a custom GitHub API base URL (primarily for testing).
func WithBaseAPI(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.baseAPI = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// NewClient builds a client with sensible defaults.
func NewClient(opts ...Option) *Client {
	client := &Client{
		owner:    defaultOwner,
		repo:     defaultRepo,
		rootPath: defaultPath,
		ref:      defaultRef,
		baseAPI:  defaultBaseAPI,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Service contains the curated information extracted from OSCAR Hub metadata.
type Service struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Creator        string `json:"creator,omitempty"`
	URL            string `json:"url,omitempty"`
	License        string `json:"license,omitempty"`
	RepositoryURL  string `json:"repository_url,omitempty"`
	MetadataSource string `json:"metadata_source,omitempty"`
}

// Warning captures non-fatal issues encountered while parsing services.
type Warning struct {
	Path string `json:"path"`
	Err  error  `json:"error"`
}

// ListResult aggregates curated services and any warnings collected.
type ListResult struct {
	Services []Service
	Warnings []Warning
}

// ListServices retrieves curated services from the configured repository.
func (c *Client) ListServices(ctx context.Context) (*ListResult, error) {
	entries, err := c.listEntries(ctx, c.rootPath)
	if err != nil {
		return nil, err
	}

	result := &ListResult{}

	for _, entry := range entries {
		if entry.Type != "dir" {
			continue
		}

		service, err := c.fetchService(ctx, entry.Path)
		if err != nil {
			if errors.Is(err, ErrMetadataNotFound) {
				continue
			}
			result.Warnings = append(result.Warnings, Warning{
				Path: entry.Path,
				Err:  err,
			})
			continue
		}
		result.Services = append(result.Services, service)
	}

	sort.Slice(result.Services, func(i, j int) bool {
		if result.Services[i].Name == result.Services[j].Name {
			return result.Services[i].Slug < result.Services[j].Slug
		}
		return result.Services[i].Name < result.Services[j].Name
	})

	return result, nil
}

type githubContent struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

func (c *Client) listEntries(ctx context.Context, repoPath string) ([]githubContent, error) {
	u := c.contentsURL(repoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, repoPath)
	}

	if res.StatusCode != http.StatusOK {
		return nil, c.readAPIError(res)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var entries []githubContent
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decoding repository contents: %w", err)
	}

	return entries, nil
}

func (c *Client) fetchService(ctx context.Context, repoPath string) (Service, error) {
	metadataPath := path.Join(repoPath, metadataFile)
	raw, err := c.getFile(ctx, metadataPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Service{}, ErrMetadataNotFound
		}
		return Service{}, err
	}

	service, err := parseROCrate(raw)
	if err != nil {
		return Service{}, fmt.Errorf("parsing metadata %s: %w", metadataPath, err)
	}

	service.Slug = path.Base(repoPath)
	service.MetadataSource = metadataPath
	if service.RepositoryURL == "" {
		service.RepositoryURL = c.composeTreeURL(repoPath)
	}
	if service.URL == "" {
		service.URL = service.RepositoryURL
	}

	return service, nil
}

func (c *Client) getFile(ctx context.Context, filePath string) ([]byte, error) {
	u := c.contentsURL(filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("User-Agent", userAgent)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return io.ReadAll(res.Body)
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, filePath)
	default:
		return nil, c.readAPIError(res)
	}
}

func (c *Client) contentsURL(repoPath string) string {
	var segments []string
	for _, segment := range strings.Split(strings.Trim(repoPath, "/"), "/") {
		if segment == "" {
			continue
		}
		segments = append(segments, url.PathEscape(segment))
	}

	owner := url.PathEscape(c.owner)
	repo := url.PathEscape(c.repo)

	builder := strings.Builder{}
	builder.Grow(len(c.baseAPI) + len(owner) + len(repo) + len(repoPath) + 32)
	builder.WriteString(c.baseAPI)
	builder.WriteString("/repos/")
	builder.WriteString(owner)
	builder.WriteString("/")
	builder.WriteString(repo)
	builder.WriteString("/contents")
	if len(segments) > 0 {
		builder.WriteString("/")
		builder.WriteString(strings.Join(segments, "/"))
	}

	if c.ref != "" {
		builder.WriteString("?ref=")
		builder.WriteString(url.QueryEscape(c.ref))
	}

	return builder.String()
}

func (c *Client) composeTreeURL(repoPath string) string {
	if repoPath == "" {
		repoPath = "."
	}

	segments := strings.Split(strings.Trim(repoPath, "/"), "/")
	joined := strings.Join(segments, "/")
	if joined == "" {
		joined = "."
	}

	ref := c.ref
	if ref == "" {
		ref = defaultRef
	}

	return fmt.Sprintf("https://github.com/%s/%s/tree/%s/%s", c.owner, c.repo, ref, joined)
}

func (c *Client) readAPIError(res *http.Response) error {
	defer io.Copy(io.Discard, res.Body) // ensure body fully read
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = res.Status
	}
	return fmt.Errorf("github api: %s (%d)", message, res.StatusCode)
}

func parseROCrate(raw []byte) (Service, error) {
	var crate struct {
		Graph []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal(raw, &crate); err != nil {
		return Service{}, fmt.Errorf("unmarshal ro-crate: %w", err)
	}
	if len(crate.Graph) == 0 {
		return Service{}, errors.New("ro-crate graph is empty")
	}

	entities := make(map[string]map[string]any, len(crate.Graph))
	for _, entity := range crate.Graph {
		id, _ := entity["@id"].(string)
		if id != "" {
			entities[id] = entity
		}
	}

	dataset := findDatasetEntity(crate.Graph)
	if dataset == nil {
		return Service{}, errors.New("dataset entity not found in ro-crate")
	}

	service := Service{}
	if name, ok := dataset["name"].(string); ok {
		service.Name = name
	}
	if desc, ok := dataset["description"].(string); ok {
		service.Description = desc
	}
	if url, ok := dataset["URL"].(string); ok {
		service.URL = url
	}

	creator := extractValue(dataset["creator"], entities)
	if creator == "" {
		creator = extractValue(dataset["author"], entities)
	}
	service.Creator = creator

	service.License = extractValue(dataset["license"], entities)

	return service, nil
}

// FetchFDL downloads the FDL definition and embeds referenced artifacts for the provided slug.
func (c *Client) FetchFDL(ctx context.Context, slug string) (*service.FDL, error) {
	repoPath := strings.Trim(path.Join(c.rootPath, slug), "/")
	if repoPath == "" {
		repoPath = slug
	}

	entries, err := c.listEntries(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	fdlFile, err := selectFDLFile(slug, entries)
	if err != nil {
		return nil, err
	}

	rawFDL, err := c.getFile(ctx, path.Join(repoPath, fdlFile))
	if err != nil {
		return nil, err
	}

	var parsed service.FDL
	if err := yaml.Unmarshal(rawFDL, &parsed); err != nil {
		return nil, fmt.Errorf("parsing FDL: %w", err)
	}

	if err := c.embedArtifacts(ctx, repoPath, &parsed); err != nil {
		return nil, err
	}

	return &parsed, nil
}

func selectFDLFile(slug string, entries []githubContent) (string, error) {
	var fallback string
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		if strings.HasSuffix(entry.Name, ".yaml") || strings.HasSuffix(entry.Name, ".yml") {
			if entry.Name == slug+".yaml" || entry.Name == slug+".yml" {
				return entry.Name, nil
			}
			if fallback == "" {
				fallback = entry.Name
			}
		}
	}
	if fallback == "" {
		return "", fmt.Errorf("fdl file not found in service directory")
	}
	return fallback, nil
}

func (c *Client) embedArtifacts(ctx context.Context, repoPath string, fdl *service.FDL) error {
	for _, element := range fdl.Functions.Oscar {
		for clusterID, svc := range element {
			if svc == nil {
				continue
			}

			scriptPath := strings.TrimSpace(svc.Script)
			if scriptPath != "" {
				clean := path.Clean(scriptPath)
				if strings.HasPrefix(clean, "..") {
					return fmt.Errorf("script path %s escapes service directory", scriptPath)
				}
				raw, err := c.getFile(ctx, path.Join(repoPath, clean))
				if err != nil {
					return fmt.Errorf("fetching script %s: %w", scriptPath, err)
				}
				svc.Script = string(raw)
			}

			svc.ClusterID = clusterID
			svc.StorageProviders = fdl.StorageProviders
			svc.Clusters = fdl.Clusters
		}
	}
	return nil
}

func findDatasetEntity(entities []map[string]any) map[string]any {
	for _, entity := range entities {
		if hasType(entity["@type"], "Dataset") {
			return entity
		}
	}
	return nil
}

func hasType(raw any, expected string) bool {
	switch v := raw.(type) {
	case string:
		return v == expected
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok && str == expected {
				return true
			}
		}
	}
	return false
}

func extractValue(raw any, entities map[string]map[string]any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return resolveEntityName(v, entities)
	case map[string]any:
		if name, ok := v["name"].(string); ok && name != "" {
			return name
		}
		if id, ok := v["@id"].(string); ok && id != "" {
			return resolveEntityName(id, entities)
		}
		return ""
	case []any:
		var names []string
		for _, item := range v {
			if name := extractValue(item, entities); name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, ", ")
	default:
		return ""
	}
}

func resolveEntityName(id string, entities map[string]map[string]any) string {
	if id == "" {
		return ""
	}

	entity, ok := entities[id]
	if !ok {
		return id
	}

	if name, ok := entity["name"].(string); ok && name != "" {
		return name
	}

	if identifier, ok := entity["identifier"].(string); ok && identifier != "" {
		return identifier
	}

	if url, ok := entity["url"].(string); ok && url != "" {
		return url
	}

	return id
}
