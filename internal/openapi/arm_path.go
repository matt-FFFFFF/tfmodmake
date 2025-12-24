package openapi

import "strings"

// azureARMInstancePathInfo parses an Azure ARM resource instance path and returns the
// fully-qualified resource type (provider + type segments) and the final {name} parameter.
//
// Examples:
// - /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets/{widgetName}
//   -> ("Microsoft.Test/widgets", "widgetName", true)
// - .../providers/Microsoft.Test/widgets/{widgetName}/child/{childName}
//   -> ("Microsoft.Test/widgets/child", "childName", true)
//
// It returns ok=false if the path is not a canonical instance path (i.e. doesn't end on a {name}
// segment with a preceding fixed type segment).
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

		// Must be followed by a {name} parameter.
		if i+1 >= len(segments) || !isPathParam(segments[i+1]) {
			break
		}

		typeSegments = append(typeSegments, seg)
		lastNameParam = strings.TrimSuffix(strings.TrimPrefix(segments[i+1], "{"), "}")
		if lastNameParam == "" {
			return "", "", false
		}
		i += 2
	}

	// Only treat this as an instance path if we consumed all segments and ended on a {name}.
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
