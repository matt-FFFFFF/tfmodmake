package openapi

import (
	"encoding/json"
	"sort"
	"strings"
)

// FormatChildrenAsText formats the children result as human-readable plain text.
func FormatChildrenAsText(result *ChildrenResult) string {
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

	sb.WriteString("Deployable child resources\n")
	if len(deployable) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, child := range deployable {
			apiVersion := child.APIVersion
			if apiVersion == "" {
				apiVersion = "(unknown)"
			}
			sb.WriteString("- " + apiVersion + "\t" + child.ResourceType + "\n")
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Filtered out\n")
	if len(filteredOut) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, child := range filteredOut {
			apiVersion := child.APIVersion
			if apiVersion == "" {
				apiVersion = "(unknown)"
			}
			reason := child.DeployabilityReason
			if reason == "" {
				reason = "(no reason)"
			}
			sb.WriteString("- " + apiVersion + "\t" + child.ResourceType + "\t" + reason + "\n")
		}
	}
	sb.WriteString("\n")

	return sb.String()
}

// FormatChildrenAsMarkdown is kept for backwards compatibility.
// It currently emits plain text (not markdown tables).
func FormatChildrenAsMarkdown(result *ChildrenResult) string {
	return FormatChildrenAsText(result)
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
