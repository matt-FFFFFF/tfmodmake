package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/matt-FFFFFF/tfmodmake/naming"
)

func defaultDiscoveryGlobsForParent(parentType string) []string {
	// When the user didn't specify -include (defaults to *.json), try a narrower
	// pattern first to avoid pulling unrelated specs from big version folders.
	// If it matches nothing, discovery code will fall back to *.json.
	last := parentType
	if idx := strings.LastIndex(parentType, "/"); idx >= 0 {
		last = parentType[idx+1:]
	}
	if last == "" {
		return []string{"*.json"}
	}
	// Common ARM spec files are PascalCase, e.g. ManagedEnvironments*.json.
	pascal := strings.ToUpper(last[:1]) + last[1:]
	return []string{pascal + "*.json", "*.json"}
}

// deriveModuleName derives a module folder name from a child resource type.
// Example: "Microsoft.App/managedEnvironments/storages" -> "storages"
func deriveModuleName(childType string) string {
	// Remove any trailing placeholder
	normalized := childType
	if strings.HasSuffix(normalized, "}") {
		if idx := strings.LastIndex(normalized, "/{"); idx != -1 {
			normalized = normalized[:idx]
		}
	}

	// Get the last segment
	lastSlash := strings.LastIndex(normalized, "/")
	if lastSlash == -1 {
		return naming.ToSnakeCase(normalized)
	}

	segment := normalized[lastSlash+1:]
	return naming.ToSnakeCase(segment)
}

// inferResourceTypeFromMainTf attempts to read the resource type from an existing main.tf file.
func inferResourceTypeFromMainTf() (string, error) {
	data, err := os.ReadFile("main.tf")
	if err != nil {
		return "", fmt.Errorf("could not read main.tf: %w", err)
	}

	// Look for type = "..." in azapi_resource block
	// This is a simple string search approach
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "type") && strings.Contains(trimmed, "=") {
			// Extract the value between quotes
			parts := strings.Split(trimmed, "\"")
			if len(parts) >= 2 {
				resourceType := parts[1]
				if strings.Contains(resourceType, "Microsoft.") {
					return resourceType, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not find resource type in main.tf")
}
