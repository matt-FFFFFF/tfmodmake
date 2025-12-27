package openapi

import "strings"

// azureARMInstancePathInfo parses an Azure ARM resource instance path and returns the
// fully-qualified resource type (provider + type segments) and the final instance segment.
//
// Examples:
// - /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets/{widgetName}
//   -> ("Microsoft.Test/widgets", "widgetName", true)
// - .../providers/Microsoft.Test/widgets/{widgetName}/child/{childName}
//   -> ("Microsoft.Test/widgets/child", "childName", true)
//
// It returns ok=false if the path is not a canonical instance path.
//
// Notes:
// - Most ARM instance paths alternate fixed type segments and parameterized instance names.
// - Some Azure resources use a fixed singleton instance name (e.g. .../blobServices/default).
// - Nested "/providers/{namespace}" segments (extension resources) are intentionally rejected by
//   this helper; it only models the common single-provider path shape used in most specs.
func azureARMInstancePathInfo(path string) (resourceType string, nameParam string, ok bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", false
	}

	segments := strings.Split(trimmed, "/")
	providersIdx := -1
	for i, seg := range segments {
		if strings.EqualFold(seg, "providers") {
			providersIdx = i
			break
		}
	}
	if providersIdx == -1 || providersIdx+1 >= len(segments) {
		return "", "", false
	}

	provider := segments[providersIdx+1]
	if provider == "" || isPathParam(provider) {
		return "", "", false
	}

	var typeSegments []string
	var lastNameParam string
	for i := providersIdx + 2; i < len(segments); {
		seg := segments[i]
		if seg == "" || isPathParam(seg) {
			return "", "", false
		}
		if strings.EqualFold(seg, "providers") {
			return "", "", false
		}

		// Must be followed by an instance segment (either a {name} parameter or a fixed singleton name).
		if i+1 >= len(segments) {
			break
		}
		instanceSeg := segments[i+1]
		if instanceSeg == "" {
			return "", "", false
		}
		if strings.EqualFold(instanceSeg, "providers") {
			return "", "", false
		}

		typeSegments = append(typeSegments, seg)
		if isPathParam(instanceSeg) {
			lastNameParam = strings.TrimSuffix(strings.TrimPrefix(instanceSeg, "{"), "}")
			if lastNameParam == "" {
				return "", "", false
			}
		} else {
			// Fixed singleton instance name (e.g. "default").
			lastNameParam = instanceSeg
		}
		i += 2
	}

	// Only treat this as an instance path if we consumed all segments and ended on an instance segment.
	if providersIdx+2+2*len(typeSegments) != len(segments) {
		return "", "", false
	}
	if len(typeSegments) == 0 {
		return "", "", false
	}
	if lastNameParam == "" {
		return "", "", false
	}

	return provider + "/" + strings.Join(typeSegments, "/"), lastNameParam, true
}

func isPathParam(seg string) bool {
	return strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
}
