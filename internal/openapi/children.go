package openapi

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var apiVersionRegex = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}(?:-preview)?)`)

// ChildResource represents a discovered child resource type.
type ChildResource struct {
	ResourceType         string   // e.g. "Microsoft.App/managedEnvironments/certificates"
	ExamplePaths         []string // Example instance paths
	Operations           []string // HTTP methods available (GET, PUT, PATCH, DELETE)
	IsDeployable         bool     // Whether this resource can be deployed
	DeployabilityReason  string   // Reason if not deployable
	APIVersion           string   // API version where this was found
}

// ChildrenResult holds the result of child resource discovery.
type ChildrenResult struct {
	Deployable   []ChildResource // Resources that can be deployed
	FilteredOut  []ChildResource // Resources that were filtered out with reasons
}

// DiscoverChildrenOptions holds options for child discovery.
type DiscoverChildrenOptions struct {
	Specs  []string // Paths or URLs to OpenAPI specs
	Parent string   // Parent resource type (e.g. "Microsoft.App/managedEnvironments")
	Depth  int      // How many levels deep to search (default 1 for direct children)
}

// DiscoverChildren discovers child resources under a parent resource type from OpenAPI specs.
// It returns deployable children and filtered-out candidates with reasons.
func DiscoverChildren(opts DiscoverChildrenOptions) (*ChildrenResult, error) {
	if len(opts.Specs) == 0 {
		return nil, fmt.Errorf("at least one spec must be provided")
	}
	if opts.Parent == "" {
		return nil, fmt.Errorf("parent resource type must be provided")
	}
	if opts.Depth <= 0 {
		opts.Depth = 1 // Default to direct children only
	}

	// Normalize parent type
	parentType := opts.Parent
	if strings.HasSuffix(parentType, "}") {
		if idx := strings.LastIndex(parentType, "/{"); idx != -1 {
			parentType = parentType[:idx]
		}
	}

	// Map to collect unique children across all specs
	// Key: resource type, Value: ChildResource
	childrenMap := make(map[string]*ChildResource)

	for _, specPath := range opts.Specs {
		doc, err := LoadSpec(specPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load spec %s: %w", specPath, err)
		}

		apiVersion := extractAPIVersion(doc, specPath)

		// Discover children in this spec
		if err := discoverChildrenInSpec(doc, parentType, opts.Depth, apiVersion, childrenMap); err != nil {
			return nil, fmt.Errorf("failed to discover children in spec %s: %w", specPath, err)
		}
	}

	// Split into deployable and filtered-out
	result := &ChildrenResult{
		Deployable:  make([]ChildResource, 0),
		FilteredOut: make([]ChildResource, 0),
	}

	for _, child := range childrenMap {
		if child.IsDeployable {
			result.Deployable = append(result.Deployable, *child)
		} else {
			result.FilteredOut = append(result.FilteredOut, *child)
		}
	}

	return result, nil
}

// extractAPIVersion extracts the API version from the OpenAPI document with fallback strategies.
// 1. Try doc.Info.Version (most common in Azure specs)
// 2. Try to extract from spec path/URL (e.g., .../2024-01-01/... or .../stable/2024-01-01/...)
// 3. Try to find api-version parameter default value
// 4. Return empty string if none found
func extractAPIVersion(doc *openapi3.T, specPath string) string {
	// Strategy 1: Use doc.Info.Version if available
	if doc != nil && doc.Info != nil && doc.Info.Version != "" {
		return doc.Info.Version
	}

	// Strategy 2: Extract from spec path/URL
	// Azure specs typically have paths like: .../stable/2024-01-01/... or .../preview/2024-01-01-preview/...
	// Pattern: YYYY-MM-DD or YYYY-MM-DD-preview
	if matches := apiVersionRegex.FindStringSubmatch(specPath); len(matches) > 1 {
		return matches[1]
	}

	// Strategy 3: Look for api-version parameter default in paths
	if doc != nil && doc.Paths != nil {
		for _, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}
			// Check operation parameters
			for _, op := range []*openapi3.Operation{pathItem.Get, pathItem.Put, pathItem.Post, pathItem.Patch, pathItem.Delete} {
				if op == nil {
					continue
				}
				for _, paramRef := range op.Parameters {
					if paramRef.Value != nil && paramRef.Value.Name == "api-version" {
						if paramRef.Value.Schema != nil && paramRef.Value.Schema.Value != nil {
							if def := paramRef.Value.Schema.Value.Default; def != nil {
								if ver, ok := def.(string); ok && ver != "" {
									return ver
								}
							}
						}
					}
				}
			}
		}
	}

	return ""
}

func discoverChildrenInSpec(doc *openapi3.T, parentType string, depth int, apiVersion string, childrenMap map[string]*ChildResource) error {
	if doc == nil || doc.Paths == nil {
		return nil
	}

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}

		// Parse the path to get resource type
		resourceType, _, ok := azureARMInstancePathInfo(path)
		if !ok {
			continue
		}

		// Check if this is a child of the parent (depth is validated inside isChildOf)
		if !isChildOf(resourceType, parentType, depth) {
			continue
		}

		// Get or create child resource entry
		child, exists := childrenMap[resourceType]
		if !exists {
			child = &ChildResource{
				ResourceType: resourceType,
				ExamplePaths: []string{path},
				Operations:   []string{},
				APIVersion:   apiVersion,
			}
			childrenMap[resourceType] = child
		} else {
			// Add path if not already present
			if !contains(child.ExamplePaths, path) {
				child.ExamplePaths = append(child.ExamplePaths, path)
			}
			// Prefer later API version. Azure API versions use YYYY-MM-DD format (with optional
			// -preview suffix), so simple string comparison is sufficient and correct.
			// Examples: "2024-01-01" < "2024-03-01", "2024-01-01" < "2025-10-02-preview"
			if apiVersion > child.APIVersion {
				child.APIVersion = apiVersion
			}
		}

		// Check operations
		hasPut := pathItem.Put != nil
		hasPatch := pathItem.Patch != nil
		hasGet := pathItem.Get != nil
		hasDelete := pathItem.Delete != nil

		if hasPut && !contains(child.Operations, "PUT") {
			child.Operations = append(child.Operations, "PUT")
		}
		if hasPatch && !contains(child.Operations, "PATCH") {
			child.Operations = append(child.Operations, "PATCH")
		}
		if hasGet && !contains(child.Operations, "GET") {
			child.Operations = append(child.Operations, "GET")
		}
		if hasDelete && !contains(child.Operations, "DELETE") {
			child.Operations = append(child.Operations, "DELETE")
		}

		// Determine deployability
		// Deployable if: has PUT or PATCH, AND has a request body schema
		if !child.IsDeployable {
			hasRequestBody := false
			if hasPut {
				hasRequestBody = hasRequestBodySchema(pathItem.Put)
			}
			if !hasRequestBody && hasPatch {
				hasRequestBody = hasRequestBodySchema(pathItem.Patch)
			}

			if hasPut || hasPatch {
				if hasRequestBody {
					child.IsDeployable = true
					child.DeployabilityReason = ""
				} else {
					child.DeployabilityReason = "No request body schema found"
				}
			} else {
				if hasGet && !hasPut && !hasPatch {
					child.DeployabilityReason = "GET-only resource"
				} else {
					child.DeployabilityReason = "No PUT or PATCH operation"
				}
			}
		}
	}

	return nil
}

// isChildOf checks if childType is a child of parentType.
func isChildOf(childType, parentType string, maxDepth int) bool {
	if !strings.HasPrefix(childType, parentType+"/") {
		return false
	}

	// Count depth
	suffix := strings.TrimPrefix(childType, parentType+"/")
	depth := strings.Count(suffix, "/") + 1

	return depth <= maxDepth
}

// hasRequestBodySchema checks if an operation has a request body with a schema.
// It handles both OpenAPI 3 RequestBody and Swagger/OpenAPI v2 body parameters,
// including $ref schemas and various JSON content types.
func hasRequestBodySchema(op *openapi3.Operation) bool {
	if op == nil {
		return false
	}

	// Check OpenAPI 3 RequestBody
	if op.RequestBody != nil {
		// Handle both direct Value and $ref
		if op.RequestBody.Ref != "" || op.RequestBody.Value != nil {
			if op.RequestBody.Value != nil {
				// Check all content types, not just "application/json"
				// Azure specs use various JSON media types: application/json, application/merge-patch+json, etc.
				for _, mediaType := range op.RequestBody.Value.Content {
					if mediaType.Schema != nil {
						// Accept both $ref and direct schema
						if mediaType.Schema.Ref != "" || mediaType.Schema.Value != nil {
							return true
						}
					}
				}
			}
		}
	}

	// Check Swagger/OpenAPI v2 body parameter
	for _, paramRef := range op.Parameters {
		// Handle both $ref and direct parameter
		if paramRef.Ref != "" || (paramRef.Value != nil && paramRef.Value.In == "body") {
			if paramRef.Value != nil && paramRef.Value.In == "body" {
				if paramRef.Value.Schema != nil {
					// Accept both $ref and direct schema
					if paramRef.Value.Schema.Ref != "" || paramRef.Value.Schema.Value != nil {
						return true
					}
				}
			}
		}
	}

	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
