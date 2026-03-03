package terraform

import (
	"sort"
	"strings"

	"github.com/matt-FFFFFF/tfmodmake/schema"
)

// extractComputedPaths traverses the ResourceSchema and extracts paths to computed (non-writable) properties.
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
func extractComputedPaths(rs *schema.ResourceSchema) []string {
	if rs == nil {
		return nil
	}

	var paths []string
	for propName, prop := range rs.Properties {
		if prop == nil {
			continue
		}
		extractComputedPathsRecursive(prop, propName, &paths)
	}

	// Apply blocklist filtering
	filtered := filterBlocklistedPaths(paths)

	// Sort for deterministic output
	sort.Strings(filtered)

	return filtered
}

// extractComputedPathsRecursive recursively traverses properties to find computed (non-writable) fields.
func extractComputedPathsRecursive(prop *schema.Property, currentPath string, paths *[]string) {
	if prop == nil {
		return
	}

	// If this property is non-writable, it's a computed value.
	if currentPath != "" && prop.ReadOnly {
		// Avoid exporting root-level id/name since those are already available as
		// azapi_resource.this.id and azapi_resource.this.name.
		if !(strings.IndexByte(currentPath, '.') == -1 && (currentPath == "id" || currentPath == "name")) {
			// Export scalars, objects, and arrays.
			if prop.IsScalar() || prop.IsContainer() {
				*paths = append(*paths, currentPath)
			}
		}
		// For leaf scalars there's nothing more to traverse.
		if prop.IsScalar() {
			return
		}
	}

	// Process object children
	if len(prop.Children) > 0 {
		for childName, child := range prop.Children {
			if child == nil {
				continue
			}

			var newPath string
			if currentPath == "" {
				newPath = childName
			} else {
				newPath = currentPath + "." + childName
			}

			extractComputedPathsRecursive(child, newPath, paths)
		}
	}
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
