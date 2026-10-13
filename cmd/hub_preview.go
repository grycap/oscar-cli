package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/grycap/oscar/v4/pkg/types"
	"gopkg.in/yaml.v3"
)

const previewRedacted = "[REDACTED]"

// printHubPreview renders the original FDL with the resolved service values,
// not the cluster-enriched HTTP request. The original source preserves only
// the fields explicitly configured by the crate rather than adding defaults.
func printHubPreview(out io.Writer, fdl *service.FDL) error {
	var data map[string]any
	if len(fdl.Source) == 0 {
		return fmt.Errorf("original FDL source is unavailable for preview")
	}
	if err := yaml.Unmarshal(fdl.Source, &data); err != nil {
		return fmt.Errorf("parsing original FDL for preview: %w", err)
	}
	if err := overlayResolvedService(data, fdl); err != nil {
		return err
	}
	redactPreview(data, "")
	preview, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("formatting resolved FDL: %w", err)
	}
	_, err = out.Write(preview)
	return err
}

func overlayResolvedService(data map[string]any, fdl *service.FDL) error {
	functions, ok := data["functions"].(map[string]any)
	if !ok {
		return fmt.Errorf("preview FDL has no functions")
	}
	entries, ok := functions["oscar"].([]any)
	if !ok {
		return fmt.Errorf("preview FDL has no OSCAR services")
	}
	for clusterID, svc := range fdl.Functions.Oscar[0] {
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			doc, ok := entry[clusterID].(map[string]any)
			if !ok {
				continue
			}
			doc["name"] = svc.Name
			doc["script"] = svc.Script
			if environment, ok := doc["environment"].(map[string]any); ok {
				for name, values := range map[string]map[string]string{"variables": svc.Environment.Vars, "secrets": svc.Environment.Secrets} {
					if original, ok := environment[name].(map[string]any); ok {
						for key := range original {
							if value, present := values[key]; present {
								original[key] = value
							}
						}
					}
				}
			}
			updateStoragePaths(doc["input"], svc.Input)
			updateStoragePaths(doc["output"], svc.Output)
			if mount, ok := doc["mount"].(map[string]any); ok {
				mount["path"] = svc.Mount.Path
			}
			functions["oscar"] = []any{map[string]any{clusterID: doc}}
			return nil
		}
	}
	return fmt.Errorf("resolved service is absent from the original FDL")
}

func updateStoragePaths(raw any, paths []types.StorageIOConfig) {
	items, ok := raw.([]any)
	if !ok {
		return
	}
	for i, item := range items {
		if i >= len(paths) {
			break
		}
		if entry, ok := item.(map[string]any); ok {
			entry["path"] = paths[i].Path
		}
	}
}

func redactPreview(value any, parent string) {
	switch fields := value.(type) {
	case map[string]any:
		for key, child := range fields {
			if parent == "environment" && key == "secrets" {
				if secrets, ok := child.(map[string]any); ok {
					for name := range secrets {
						secrets[name] = previewRedacted
					}
				}
				continue
			}
			if isSensitivePreviewKey(key) {
				fields[key] = previewRedacted
				continue
			}
			redactPreview(child, key)
		}
	case []any:
		for _, child := range fields {
			redactPreview(child, parent)
		}
	}
}

func isSensitivePreviewKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "SECRET", "TOKEN", "API_KEY", "ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
