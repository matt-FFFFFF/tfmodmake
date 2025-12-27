package terraform

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// extractComputedPaths traverses the schema and extracts paths to computed (non-writable) properties.
// It returns a sorted list of JSON paths suitable for azapi_resource.response_export_values.
//
// The function applies a blocklist to filter out noisy fields:
//   - Array-indexed paths (containing "[")
//   - Status fields (containing ".status.")
//   - Provisioning error fields (containing ".provisioningError.")
//   - eTag fields (case-insensitive)
//   - Timestamp fields (createdAt, lastModified, etc.)
//
// These blocklist rules help generate a useful default set of exports that module authors
// can trim to their specific needs.
func extractComputedPaths(schema *openapi3.Schema) []string {
	if schema == nil {
		return nil
	}

	var paths []string
	extractComputedPathsRecursive(schema, "", &paths, make(map[*openapi3.Schema]struct{}))

	// Apply blocklist filtering
	filtered := filterBlocklistedPaths(paths)

	// Sort for deterministic output
	sort.Strings(filtered)

	return filtered
}

// extractComputedPathsRecursive recursively traverses the schema to find computed (non-writable) properties.
// Uses a recursion stack to prevent infinite loops while allowing the same schema to be
// visited multiple times if it appears in different paths.
func extractComputedPathsRecursive(schema *openapi3.Schema, currentPath string, paths *[]string, visited map[*openapi3.Schema]struct{}) {
	if schema == nil {
		return
	}

	// Prevent infinite recursion from circular references using a stack-based approach
	// Mark as visited before descending, unmark on return
	if _, seen := visited[schema]; seen {
		return
	}
	visited[schema] = struct{}{}
	defer delete(visited, schema)

	// If this property is non-writable, it's a computed value.
	if currentPath != "" && !isWritableProperty(schema) {
		// Avoid exporting root-level id/name since those are already available as
		// azapi_resource.this.id and azapi_resource.this.name.
		if !(strings.IndexByte(currentPath, '.') == -1 && (currentPath == "id" || currentPath == "name")) {
			// Export scalars, objects, and arrays. Objects/arrays are useful when the
			// schema doesn't flag nested leaves as readOnly but the provider returns
			// a computed structure.
			if isLeafScalar(schema) || isContainer(schema) {
				*paths = append(*paths, currentPath)
			}
		}
		// For leaf scalars there's nothing more to traverse.
		if isLeafScalar(schema) {
			return
		}
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

			extractComputedPathsRecursive(propRef.Value, newPath, paths, visited)
		}
	}

	// Process allOf schemas (inheritance)
	for _, allOfRef := range schema.AllOf {
		if allOfRef == nil || allOfRef.Value == nil {
			continue
		}
		extractComputedPathsRecursive(allOfRef.Value, currentPath, paths, visited)
	}
}

// isLeafScalar returns true if the schema represents a scalar type (string, number, integer, boolean)
// and is not an object or array. Checks all types in the schema.Type array.
func isLeafScalar(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil {
		return false
	}

	types := *schema.Type
	if len(types) == 0 {
		return false
	}

	// Check all types in the array (e.g., ["null", "string"])
	// Return true if any type is a scalar (ignoring "null")
	hasScalar := false
	for _, typ := range types {
		if typ == "string" || typ == "number" || typ == "integer" || typ == "boolean" {
			hasScalar = true
		} else if typ != "null" {
			// If there's a non-scalar, non-null type (object, array), it's not a leaf scalar
			return false
		}
	}

	return hasScalar
}

func isContainer(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil {
		return false
	}
	for _, typ := range *schema.Type {
		if typ == "object" || typ == "array" {
			return true
		}
	}
	return false
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

	// Block status-related paths (both root-level and nested)
	// Matches: "status", "status.phase", "properties.status.ready"
	if path == "status" || strings.HasPrefix(path, "status.") || strings.Contains(path, ".status.") {
		return true
	}

	// Block provisioning error paths (both root-level and nested)
	// Matches: "provisioningError", "provisioningError.code", "properties.provisioningError.message"
	if path == "provisioningError" || strings.HasPrefix(path, "provisioningError.") || strings.Contains(path, ".provisioningError.") {
		return true
	}

	// Block eTag fields (case-insensitive)
	// Matches both standalone "eTag" and paths ending with ".eTag"
	lowerPath := strings.ToLower(path)
	if lowerPath == "etag" || strings.HasSuffix(lowerPath, ".etag") {
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
