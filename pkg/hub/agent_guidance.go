package hub

import (
	"context"
	"errors"
	"fmt"

	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/grycap/oscar-cli/v2/pkg/service"
)

// MaterializeAgentGuidance resolves the local guidance declared by OSCAR Agent
// Profile 1.1. Only variables already declared in the FDL are changed.
func MaterializeAgentGuidance(fdl *service.FDL, raw []byte, readFile func(string) ([]byte, error)) error {
	crate, err := ParseROCrate(raw)
	if err != nil {
		return err
	}
	root, err := crate.datasetNode()
	if err != nil {
		return err
	}
	if !hasType(root["@type"], "Agent") {
		return nil
	}
	profile := extractIDs(root["conformsTo"])
	if len(profile) != 1 || profile[0] != "https://github.com/grycap/oscar-agents/tree/devel/profiles/oscar-agent-1.1" {
		return fmt.Errorf("unsupported OSCAR agent profile: %v", profile)
	}
	parts := make(map[string]bool)
	for _, id := range extractIDs(root["hasPart"]) {
		parts[id] = true
	}
	resolve := func(id, kind string) (string, error) {
		if !parts[id] || !validGuidancePath(id) {
			return "", fmt.Errorf("%s %q is not a local crate part", kind, id)
		}
		entity := crate.entity(id)
		if !hasType(entity["@type"], kind) || !hasType(entity["@type"], "File") || entity["encodingFormat"] != "text/markdown" {
			return "", fmt.Errorf("%s %q is not a Markdown %s file", kind, id, kind)
		}
		data, err := readFile(id)
		if err != nil {
			return "", fmt.Errorf("reading %s %q: %w", kind, id, err)
		}
		if !utf8.Valid(data) {
			return "", fmt.Errorf("%s %q is not UTF-8", kind, id)
		}
		return string(data), nil
	}
	var soul, skills string
	needSoul, needSkills := false, false
	for _, entry := range fdl.Functions.Oscar {
		for _, svc := range entry {
			if svc == nil {
				continue
			}
			_, s := svc.Environment.Vars["AGENT_SOUL"]
			_, k := svc.Environment.Vars["AGENT_SKILLS"]
			needSoul = needSoul || s
			needSkills = needSkills || k
		}
	}
	if needSoul {
		ids := extractIDs(root["agentSoul"])
		if len(ids) != 1 {
			return fmt.Errorf("agentSoul must reference exactly one local AgentSoul")
		}
		soul, err = resolve(ids[0], "AgentSoul")
		if err != nil {
			return err
		}
	}
	if needSkills {
		ids := extractIDs(root["agentSkills"])
		if _, present := root["agentSkills"]; present && len(ids) == 0 {
			return fmt.Errorf("agentSkills must reference local AgentSkill files")
		}
		sort.Strings(ids)
		values := make([]string, 0, len(ids))
		for _, id := range ids {
			value, err := resolve(id, "AgentSkill")
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		skills = strings.Join(values, "\n\n")
	}
	for _, entry := range fdl.Functions.Oscar {
		for _, svc := range entry {
			if svc == nil {
				continue
			}
			if _, ok := svc.Environment.Vars["AGENT_SOUL"]; ok {
				svc.Environment.Vars["AGENT_SOUL"] = soul
			}
			if _, ok := svc.Environment.Vars["AGENT_SKILLS"]; ok {
				svc.Environment.Vars["AGENT_SKILLS"] = skills
			}
		}
	}
	return nil
}

func validGuidancePath(id string) bool {
	if id == "" || strings.Contains(id, "\\") || strings.Contains(id, ":") || strings.HasPrefix(id, "/") || path.Clean(id) != id {
		return false
	}
	for _, part := range strings.Split(id, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return strings.HasSuffix(id, ".md")
}

// MaterializeLocalAgentGuidance reads metadata next to the selected FDL.
// Ordinary Hub crates without metadata retain their existing behavior.
func MaterializeLocalAgentGuidance(localRoot, slug string, fdl *service.FDL) error {
	raw, baseDir, err := loadLocalMetadata(localRoot, slug)
	if err != nil {
		// Metadata is optional for ordinary FDLs, but malformed metadata is not.
		if errors.Is(err, ErrMetadataNotFound) {
			return nil
		}
		return err
	}
	base, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return err
	}
	return MaterializeAgentGuidance(fdl, raw, func(id string) ([]byte, error) {
		if !validGuidancePath(id) {
			return nil, fmt.Errorf("invalid guidance path %q", id)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(baseDir, filepath.FromSlash(id)))
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(base, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("guidance path %q escapes crate", id)
		}
		return os.ReadFile(resolved)
	})
}

// MaterializeRemoteAgentGuidance reads only crate-local references from the
// same repository and Git ref used for the FDL.
func (c *Client) MaterializeRemoteAgentGuidance(ctx context.Context, slug string, fdl *service.FDL) error {
	repoPath := c.serviceRepoPath(slug)
	raw, err := c.getFile(ctx, path.Join(repoPath, metadataFile))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return MaterializeAgentGuidance(fdl, raw, func(id string) ([]byte, error) {
		if !validGuidancePath(id) {
			return nil, fmt.Errorf("invalid guidance path %q", id)
		}
		return c.getFile(ctx, path.Join(repoPath, id))
	})
}
