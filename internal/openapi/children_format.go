package openapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FormatChildrenAsMarkdown formats the children result as a markdown table.
func FormatChildrenAsMarkdown(result *ChildrenResult) string {
	if result == nil {
		return "No results\n"
	}

	var sb strings.Builder

	// Sort deployable children by resource type
	deployable := make([]ChildResource, len(result.Deployable))
	copy(deployable, result.Deployable)
	sort.Slice(deployable, func(i, j int) bool {
		return deployable[i].ResourceType < deployable[j].ResourceType
	})

	// Sort filtered out by resource type
	filteredOut := make([]ChildResource, len(result.FilteredOut))
	copy(filteredOut, result.FilteredOut)
	sort.Slice(filteredOut, func(i, j int) bool {
		return filteredOut[i].ResourceType < filteredOut[j].ResourceType
	})

	sb.WriteString("# Deployable Child Resources\n\n")
	if len(deployable) == 0 {
		sb.WriteString("*No deployable child resources found.*\n\n")
	} else {
		sb.WriteString("| Resource Type | Operations | API Version | Example Path |\n")
		sb.WriteString("|--------------|------------|-------------|-------------|\n")
		for _, child := range deployable {
			ops := strings.Join(child.Operations, ", ")
			examplePath := ""
			if len(child.ExamplePaths) > 0 {
				examplePath = child.ExamplePaths[0]
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				child.ResourceType, ops, child.APIVersion, examplePath))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("# Filtered Out\n\n")
	if len(filteredOut) == 0 {
		sb.WriteString("*No resources were filtered out.*\n\n")
	} else {
		sb.WriteString("| Resource Type | Reason | Operations | API Version |\n")
		sb.WriteString("|--------------|--------|------------|-------------|\n")
		for _, child := range filteredOut {
			ops := strings.Join(child.Operations, ", ")
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				child.ResourceType, child.DeployabilityReason, ops, child.APIVersion))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatChildrenAsJSON formats the children result as JSON.
func FormatChildrenAsJSON(result *ChildrenResult) (string, error) {
	if result == nil {
		return "{}", nil
	}

	// Sort for consistent output
	deployable := make([]ChildResource, len(result.Deployable))
	copy(deployable, result.Deployable)
	sort.Slice(deployable, func(i, j int) bool {
		return deployable[i].ResourceType < deployable[j].ResourceType
	})

	filteredOut := make([]ChildResource, len(result.FilteredOut))
	copy(filteredOut, result.FilteredOut)
	sort.Slice(filteredOut, func(i, j int) bool {
		return filteredOut[i].ResourceType < filteredOut[j].ResourceType
	})

	output := struct {
		Deployable  []ChildResource `json:"deployable"`
		FilteredOut []ChildResource `json:"filtered_out"`
	}{
		Deployable:  deployable,
		FilteredOut: filteredOut,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
