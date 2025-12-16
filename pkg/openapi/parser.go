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

	// Strategy: Look for a path that ends with the resource type (ignoring {name})
	// Azure paths usually look like: .../providers/Microsoft.ContainerService/managedClusters/{resourceName}

	var bestMatchSchema *openapi3.Schema

	for path, pathItem := range doc.Paths.Map() {
		// Check if path corresponds to the resource type
		// We look for .../providers/<resourceType>/{name} or just .../<resourceType>
		// Simple heuristic: check if path contains the resource type
		if strings.Contains(strings.ToLower(path), strings.ToLower(resourceType)) {
			// We prefer the PUT operation for resource creation
			if pathItem.Put != nil && pathItem.Put.RequestBody != nil {
				content := pathItem.Put.RequestBody.Value.Content
				if jsonContent, ok := content["application/json"]; ok {
					if jsonContent.Schema != nil {
						bestMatchSchema = jsonContent.Schema.Value
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
	}

	if bestMatchSchema != nil {
		return bestMatchSchema, nil
	}

	// Fallback: Try to find in definitions/schemas if the resourceType matches a schema name
	// This is less reliable as schema names are arbitrary, but sometimes they match.
	// For Azure, resourceType "Microsoft.ContainerService/managedClusters" might not match "ManagedCluster" directly without mapping.

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
		current = prop.Value
	}
	return current, nil
}
