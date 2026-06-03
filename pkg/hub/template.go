package hub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	templateLinePattern    = regexp.MustCompile(`(?m)^([ \t]*)\{\{\s*(file\s+"([^"]+)"|agentSkills)\s*\}\}\s*$`)
	unsupportedLinePattern = regexp.MustCompile(`(?m)^[ \t]*\{\{.*\}\}\s*$`)
)

type templateResolver interface {
	ReadFile(ref string) ([]byte, error)
	ReadURL(ref string) ([]byte, error)
}

func renderFDLTemplate(raw []byte, crate *ROCrate, resolver templateResolver) ([]byte, error) {
	text := string(raw)
	if !strings.Contains(text, "{{") {
		return raw, nil
	}

	var renderErr error
	rendered := templateLinePattern.ReplaceAllStringFunc(text, func(match string) string {
		if renderErr != nil {
			return match
		}

		parts := templateLinePattern.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}

		indent := parts[1]
		expr := strings.TrimSpace(parts[2])

		var content string
		switch {
		case strings.HasPrefix(expr, "file "):
			rawFile, err := resolver.ReadFile(parts[3])
			if err != nil {
				renderErr = err
				return match
			}
			content = string(rawFile)
		case expr == "agentSkills":
			if crate == nil {
				renderErr = fmt.Errorf("agentSkills template requires ro-crate metadata")
				return match
			}
			skills, err := renderAgentSkills(crate, resolver)
			if err != nil {
				renderErr = err
				return match
			}
			content = skills
		default:
			renderErr = fmt.Errorf("unsupported template expression %q", expr)
			return match
		}

		return indentContent(content, indent)
	})

	if renderErr != nil {
		return nil, renderErr
	}
	if unsupportedLinePattern.MatchString(rendered) {
		return nil, fmt.Errorf("unsupported or inline template expression in FDL")
	}
	return []byte(rendered), nil
}

func renderAgentSkills(crate *ROCrate, resolver templateResolver) (string, error) {
	dataset, err := crate.datasetNode()
	if err != nil {
		return "", err
	}

	skillIDs := extractIDs(dataset["agentSkills"])
	if len(skillIDs) == 0 {
		return "", nil
	}

	var blocks []string
	for _, skillID := range skillIDs {
		node := crate.entity(skillID)
		if node == nil {
			return "", fmt.Errorf("agent skill %s is not defined in ro-crate metadata", skillID)
		}

		block, err := renderAgentSkill(skillID, node, resolver)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(block) != "" {
			blocks = append(blocks, block)
		}
	}

	return strings.Join(blocks, "\n\n---\n\n"), nil
}

func renderAgentSkill(skillID string, node map[string]interface{}, resolver templateResolver) (string, error) {
	name := readString(node, "name")
	if name == "" {
		name = skillID
	}

	source := readString(node, "skillSource")
	contentRef := readString(node, "contentUrl")
	if contentRef == "" && !isHTTPRef(skillID) {
		contentRef = skillID
	}

	switch source {
	case "", "local":
		if contentRef == "" {
			return "", fmt.Errorf("local agent skill %s does not define a file reference", skillID)
		}
		raw, err := resolver.ReadFile(contentRef)
		if err != nil {
			return "", fmt.Errorf("reading agent skill %s: %w", skillID, err)
		}
		return skillBlock(name, contentRef, string(raw)), nil
	case "url":
		ref := readString(node, "url")
		if ref == "" {
			ref = skillID
		}
		raw, err := resolver.ReadURL(ref)
		if err != nil {
			return "", fmt.Errorf("reading agent skill %s: %w", skillID, err)
		}
		return skillBlock(name, ref, string(raw)), nil
	case "marketplace":
		ref := readString(node, "url")
		if ref == "" {
			ref = skillID
		}
		description := readString(node, "description")
		content := strings.TrimSpace(description)
		if content != "" {
			content += "\n\n"
		}
		content += "Use the external marketplace skill referenced above when resolving the agent instructions."
		return skillBlock(name, ref, content), nil
	default:
		return "", fmt.Errorf("unsupported skillSource %q for agent skill %s", source, skillID)
	}
}

func skillBlock(name, source, content string) string {
	return fmt.Sprintf("# Skill: %s\nSource: %s\n\n%s", name, source, strings.TrimSpace(content))
}

func indentContent(content, indent string) string {
	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return indent
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = indent + strings.TrimRight(line, "\r")
	}
	return strings.Join(lines, "\n")
}

func isHTTPRef(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

type remoteTemplateResolver struct {
	ctx      context.Context
	client   *Client
	repoPath string
}

func (r remoteTemplateResolver) ReadFile(ref string) ([]byte, error) {
	resolved, err := resolveRepoPath(r.repoPath, ref)
	if err != nil {
		return nil, err
	}
	return r.client.getFile(r.ctx, resolved)
}

func (r remoteTemplateResolver) ReadURL(ref string) ([]byte, error) {
	return readURL(r.ctx, r.client.httpClient, ref)
}

type localTemplateResolver struct {
	rootDir string
	baseDir string
}

func (r localTemplateResolver) ReadFile(ref string) ([]byte, error) {
	resolved, err := resolveLocalPath(r.rootDir, r.baseDir, ref)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (r localTemplateResolver) ReadURL(ref string) ([]byte, error) {
	return readURL(context.Background(), http.DefaultClient, ref)
}

func resolveRepoPath(repoPath, ref string) (string, error) {
	if isHTTPRef(ref) {
		return "", fmt.Errorf("expected repository file path, got URL %s", ref)
	}
	if strings.HasPrefix(ref, "/") {
		return "", fmt.Errorf("absolute repository path %s is not allowed", ref)
	}
	clean := path.Clean(path.Join(repoPath, ref))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("path %s escapes repository root", ref)
	}
	return clean, nil
}

func resolveLocalPath(rootDir, baseDir, ref string) (string, error) {
	if isHTTPRef(ref) {
		return "", fmt.Errorf("expected local file path, got URL %s", ref)
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("absolute local path %s is not allowed", ref)
	}

	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(ref)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes local root", ref)
	}
	return targetAbs, nil
}

func inferLocalPackageRoot(localRoot, baseDir string) string {
	current := filepath.Clean(localRoot)
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return current
	}

	for {
		currentAbs, err := filepath.Abs(current)
		if err != nil {
			return filepath.Clean(localRoot)
		}
		rel, err := filepath.Rel(currentAbs, baseAbs)
		if err != nil {
			return filepath.Clean(localRoot)
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if looksLikeAgentPackageRoot(currentAbs, baseAbs) {
				return currentAbs
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(localRoot)
		}
		current = parent
	}
}

func looksLikeAgentPackageRoot(rootAbs, baseAbs string) bool {
	if _, err := os.Stat(filepath.Join(rootAbs, "frameworks")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(rootAbs, "skills")); err != nil {
		return false
	}

	rel, err := filepath.Rel(rootAbs, baseAbs)
	if err != nil {
		return false
	}
	return rel == "agents" || strings.HasPrefix(rel, "agents"+string(filepath.Separator))
}

func readURL(ctx context.Context, client *http.Client, ref string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		io.Copy(io.Discard, res.Body)
		return nil, fmt.Errorf("fetching %s returned %s", ref, res.Status)
	}
	return io.ReadAll(res.Body)
}
