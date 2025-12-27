package openapi

import (
	"encoding/json"
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

	padRight := func(s string, width int) string {
		if len(s) >= width {
			return s
		}
		return s + strings.Repeat(" ", width-len(s))
	}

	// Keep API version column visually stable in terminals.
	// "2025-10-02-preview" is 18 chars; pad shorter versions up to at least that width.
	apiVersionWidth := 18
	for _, child := range deployable {
		if len(child.APIVersion) > apiVersionWidth {
			apiVersionWidth = len(child.APIVersion)
		}
	}
	for _, child := range filteredOut {
		if len(child.APIVersion) > apiVersionWidth {
			apiVersionWidth = len(child.APIVersion)
		}
	}

	sb.WriteString("# Deployable Child Resources\n\n")
	if len(deployable) == 0 {
		sb.WriteString("*No deployable child resources found.*\n\n")
	} else {
		sb.WriteString("| API Version" + strings.Repeat(" ", apiVersionWidth-len("API Version")) + " | Resource Type |\n")
		sb.WriteString("|" + strings.Repeat("-", apiVersionWidth+2) + "|--------------|\n")
		for _, child := range deployable {
			apiVersion := padRight(child.APIVersion, apiVersionWidth)
			sb.WriteString("| " + apiVersion + " | " + child.ResourceType + " |\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("# Filtered Out\n\n")
	if len(filteredOut) == 0 {
		sb.WriteString("*No resources were filtered out.*\n\n")
	} else {
		sb.WriteString("| API Version" + strings.Repeat(" ", apiVersionWidth-len("API Version")) + " | Resource Type | Reason |\n")
		sb.WriteString("|" + strings.Repeat("-", apiVersionWidth+2) + "|--------------|--------|\n")
		for _, child := range filteredOut {
			apiVersion := padRight(child.APIVersion, apiVersionWidth)
			sb.WriteString("| " + apiVersion + " | " + child.ResourceType + " | " + child.DeployabilityReason + " |\n")
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
