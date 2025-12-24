package terraform

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// extractReadOnlyPaths traverses the schema and extracts paths to readOnly leaf scalar properties.
// It returns a sorted list of JSON paths suitable for azapi_resource.response_export_values.
func extractReadOnlyPaths(schema *openapi3.Schema) []string {
	if schema == nil {
		return nil
	}

	var paths []string
	extractReadOnlyPathsRecursive(schema, "", &paths, make(map[*openapi3.Schema]struct{}))

	// Apply blocklist filtering
	filtered := filterBlocklistedPaths(paths)

	// Sort for deterministic output
	sort.Strings(filtered)

	return filtered
}

// extractReadOnlyPathsRecursive recursively traverses the schema to find readOnly leaf scalars.
func extractReadOnlyPathsRecursive(schema *openapi3.Schema, currentPath string, paths *[]string, visited map[*openapi3.Schema]struct{}) {
	if schema == nil {
		return
	}

	// Prevent infinite recursion from circular references
	if _, seen := visited[schema]; seen {
		return
	}
	visited[schema] = struct{}{}

	// Check if this is a readOnly leaf scalar
	if schema.ReadOnly && isLeafScalar(schema) {
		if currentPath != "" {
			*paths = append(*paths, currentPath)
		}
		return
	}

	// Process object properties
	if len(schema.Properties) > 0 {
		for propName, propRef := range schema.Properties {
			if propRef == nil || propRef.Value == nil {
				continue
			}

			var newPath string
			if currentPath == "" {
				newPath = propName
			} else {
				newPath = currentPath + "." + propName
			}

			extractReadOnlyPathsRecursive(propRef.Value, newPath, paths, visited)
		}
	}

	// Process allOf schemas (inheritance)
	for _, allOfRef := range schema.AllOf {
		if allOfRef == nil || allOfRef.Value == nil {
			continue
		}
		extractReadOnlyPathsRecursive(allOfRef.Value, currentPath, paths, visited)
	}
}

// isLeafScalar returns true if the schema represents a scalar type (string, number, integer, boolean)
// and is not an object or array.
func isLeafScalar(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil {
		return false
	}

	types := *schema.Type
	if len(types) == 0 {
		return false
	}

	typ := types[0]
	return typ == "string" || typ == "number" || typ == "integer" || typ == "boolean"
}

// filterBlocklistedPaths removes paths that match the blocklist criteria:
// - Contains "[" (array-indexed paths)
// - Contains ".status."
// - Contains ".provisioningError."
// - Ends with "eTag" or "etag"
// - Looks like a timestamp field
func filterBlocklistedPaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))

	for _, path := range paths {
		if shouldBlockPath(path) {
			continue
		}
		filtered = append(filtered, path)
	}

	return filtered
}

// shouldBlockPath returns true if the path should be excluded from response_export_values.
func shouldBlockPath(path string) bool {
	// Block array-indexed paths
	if strings.Contains(path, "[") {
		return true
	}

	// Block status-related paths
	if strings.Contains(path, ".status.") {
		return true
	}

	// Block provisioning error paths
	if strings.Contains(path, ".provisioningError.") {
		return true
	}

	// Block eTag fields (case-insensitive)
	lowerPath := strings.ToLower(path)
	if strings.HasSuffix(lowerPath, "etag") || strings.HasSuffix(lowerPath, ".etag") {
		return true
	}

	// Block timestamp-looking fields
	if isTimestampField(path) {
		return true
	}

	return false
}

// isTimestampField returns true if the path looks like a timestamp field.
func isTimestampField(path string) bool {
	lowerPath := strings.ToLower(path)

	// Common timestamp field patterns
	timestampSuffixes := []string{
		"timestamp",
		"createdat",
		"updatedat",
		"deletedat",
		"modifiedat",
		"createdtime",
		"modifiedtime",
		"lastupdated",
		"lastmodified",
	}

	for _, suffix := range timestampSuffixes {
		if strings.HasSuffix(lowerPath, suffix) {
			return true
		}
	}

	return false
}
