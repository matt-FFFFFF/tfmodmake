// Package openapi provides functions to parse OpenAPI specifications and extract resource schemas.
package openapi

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// LoadSpec loads the OpenAPI specification from a file path or URL.
func LoadSpec(path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	u, err := url.Parse(path)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return loader.LoadFromURI(u)
	}

	return loader.LoadFromFile(path)
}

// FindResource identifies the schema for the specified resource type.
// It looks for a path containing the resource type and returns the schema
// for the PUT request body.
func FindResource(doc *openapi3.T, resourceType string) (*openapi3.Schema, error) {
	// Normalize resource type for search
	// e.g. Microsoft.ContainerService/managedClusters

	// If the resource type contains a placeholder (e.g. {resourceName}), strip it
	// to match against the path regardless of the parameter name used in the spec.
	searchType := resourceType
	if strings.HasSuffix(searchType, "}") {
		if idx := strings.LastIndex(searchType, "/{"); idx != -1 {
			searchType = searchType[:idx]
		}
	}

	// Strategy: Look for a path that ends with the resource type (ignoring {name})
	// Azure paths usually look like: .../providers/Microsoft.ContainerService/managedClusters/{resourceName}

	var bestMatchSchema *openapi3.Schema

	for path, pathItem := range doc.Paths.Map() {
		// Check if path corresponds to the resource type
		// We look for .../providers/<resourceType>/{name} or just .../<resourceType>

		lowerPath := strings.ToLower(path)
		lowerResourceType := strings.ToLower(searchType)
		idx := strings.Index(lowerPath, lowerResourceType)

		if idx != -1 {
			// Ensure we matched a full path segment (start)
			if idx > 0 && lowerPath[idx-1] != '/' {
				continue
			}

			// Check suffix
			suffix := lowerPath[idx+len(lowerResourceType):]

			// Ensure we matched a full path segment (end)
			if suffix != "" && suffix[0] != '/' {
				continue
			}

			// Check for child resources
			// We allow at most one path segment after the resource type (the resource name)
			// Suffix is either "" or "/{name}" or "/{name}/child..."

			segments := 0
			if suffix != "" {
				// Remove leading slash
				trimmed := suffix[1:]
				if trimmed != "" {
					// Count segments
					segments = strings.Count(trimmed, "/") + 1
				}
			}

			if segments > 1 {
				continue
			}

			// We prefer the PUT operation for resource creation
			if pathItem.Put != nil {
				var schema *openapi3.Schema

				// Check RequestBody (OpenAPI 3)
				if pathItem.Put.RequestBody != nil && pathItem.Put.RequestBody.Value != nil {
					content := pathItem.Put.RequestBody.Value.Content
					if jsonContent, ok := content["application/json"]; ok {
						if jsonContent.Schema != nil {
							schema = jsonContent.Schema.Value
						}
					}
				}

				// Check Parameters (Swagger 2.0 compatibility)
				if schema == nil {
					for _, paramRef := range pathItem.Put.Parameters {
						if paramRef.Value != nil && paramRef.Value.In == "body" && paramRef.Value.Schema != nil {
							schema = paramRef.Value.Schema.Value
							break
						}
					}
				}

				if schema != nil {
					bestMatchSchema = schema
					// If we find a direct match, we might want to stop or keep looking for a better one?
					// For now, let's take the first one that looks like a resource creation.
					// Usually the path ending in /{resourceName} is the one.
					if strings.HasSuffix(path, "}") { // ends with parameter
						return bestMatchSchema, nil
					}
				}
			}
		}
	}

	if bestMatchSchema != nil {
		return bestMatchSchema, nil
	}

	// Fallback: Try to find in definitions/schemas if the resourceType matches a schema name
	// This is less reliable as schema names are arbitrary, but sometimes they match.
	// For Azure, resourceType "Microsoft.ContainerService/managedClusters" might not match "ManagedCluster" directly without mapping.
	parts := strings.Split(searchType, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Try exact match, case-insensitive match, and singularized match
		candidates := []string{name, strings.TrimSuffix(name, "s")}

		if doc.Components != nil && doc.Components.Schemas != nil {
			for schemaName, schemaRef := range doc.Components.Schemas {
				for _, candidate := range candidates {
					if strings.EqualFold(schemaName, candidate) {
						if schemaRef.Value != nil {
							return schemaRef.Value, nil
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("resource type %s not found in spec", resourceType)
}

// NavigateSchema traverses the schema properties based on the dot-separated path.
func NavigateSchema(schema *openapi3.Schema, path string) (*openapi3.Schema, error) {
	if path == "" {
		return schema, nil
	}
	parts := strings.Split(path, ".")
	current := schema
	for _, part := range parts {
		if current.Properties == nil {
			return nil, fmt.Errorf("path segment %s not found: schema has no properties", part)
		}
		prop, ok := current.Properties[part]
		if !ok {
			return nil, fmt.Errorf("property %s not found", part)
		}
		if prop.Value == nil {
			return nil, fmt.Errorf("property %s has nil schema", part)
		}
		if prop.Value.ReadOnly {
			return nil, nil // Indicate read-only property
		}
		current = prop.Value
	}
	return current, nil
}
