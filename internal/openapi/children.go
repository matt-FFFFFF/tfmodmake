package openapi

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

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

		apiVersion := ""
		if doc.Info != nil {
			apiVersion = doc.Info.Version
		}

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

func discoverChildrenInSpec(doc *openapi3.T, parentType string, depth int, apiVersion string, childrenMap map[string]*ChildResource) error {
	if doc == nil || doc.Paths == nil {
		return nil
	}

	parentSegments := strings.Count(parentType, "/")

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}

		// Parse the path to get resource type
		resourceType, _, ok := azureARMInstancePathInfo(path)
		if !ok {
			continue
		}

		// Check if this is a child of the parent
		if !isChildOf(resourceType, parentType, depth) {
			continue
		}

		// Check depth constraint
		childSegments := strings.Count(resourceType, "/")
		if childSegments != parentSegments+depth {
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
			// Prefer later API version (simple string comparison works for most Azure versions)
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
func hasRequestBodySchema(op *openapi3.Operation) bool {
	if op == nil {
		return false
	}

	// Check OpenAPI 3 RequestBody
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		content := op.RequestBody.Value.Content
		if jsonContent, ok := content["application/json"]; ok {
			if jsonContent.Schema != nil && jsonContent.Schema.Value != nil {
				return true
			}
		}
	}

	// Check Swagger/OpenAPI v2 body parameter
	for _, paramRef := range op.Parameters {
		if paramRef.Value != nil && paramRef.Value.In == "body" && paramRef.Value.Schema != nil {
			return true
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
