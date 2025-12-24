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
	var doc *openapi3.T
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		doc, err = loader.LoadFromURI(u)
	} else {
		doc, err = loader.LoadFromFile(path)
	}
	if err != nil {
		return nil, err
	}

	// Ensure all $ref pointers are resolved so downstream helpers can reliably access
	// schemas/parameters even when specs use shared definitions.
	if err := loader.ResolveRefsIn(doc, nil); err != nil {
		return nil, fmt.Errorf("resolving refs: %w", err)
	}

	return doc, nil
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

	// Strategy: Prefer canonical ARM instance paths (ending in a {name} after the resource type).
	// Azure paths usually look like: .../providers/Microsoft.ContainerService/managedClusters/{resourceName}
	// Keep the previous substring heuristic as a fallback for non-standard specs.

	var bestMatchSchema *openapi3.Schema

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil || pathItem.Put == nil {
			continue
		}

		matched := false
		if parsedType, _, ok := azureARMInstancePathInfo(path); ok {
			matched = strings.EqualFold(parsedType, searchType)
		} else {
			lowerPath := strings.ToLower(path)
			lowerResourceType := strings.ToLower(searchType)
			idx := strings.Index(lowerPath, lowerResourceType)
			if idx == -1 {
				continue
			}
			// Ensure we matched a full path segment (start).
			if idx > 0 && lowerPath[idx-1] != '/' {
				continue
			}
			suffix := lowerPath[idx+len(lowerResourceType):]
			// Ensure we matched a full path segment (end).
			if suffix != "" && suffix[0] != '/' {
				continue
			}
			// Reject child resources (allow at most one segment after resource type).
			segments := 0
			if suffix != "" {
				trimmed := suffix[1:]
				if trimmed != "" {
					segments = strings.Count(trimmed, "/") + 1
				}
			}
			if segments > 1 {
				continue
			}
			matched = true
		}

		if !matched {
			continue
		}

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

		// Fallback for Swagger/OpenAPI v2 specs, which model request bodies as
		// a body parameter instead of an OpenAPI v3 RequestBody.
		// Azure REST API specs can still contain these in older/preview specs.
		if schema == nil {
			for _, paramRef := range pathItem.Put.Parameters {
				if paramRef.Value != nil && paramRef.Value.In == "body" && paramRef.Value.Schema != nil {
					schema = paramRef.Value.Schema.Value
					break
				}
			}
		}

		if schema == nil {
			continue
		}

		bestMatchSchema = schema
		// Prefer instance paths ending in /{name} when present.
		if _, _, ok := azureARMInstancePathInfo(path); ok {
			return bestMatchSchema, nil
		}
		if strings.HasSuffix(path, "}") {
			return bestMatchSchema, nil
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

// FindResourceNameSchema identifies the schema for the resource name path parameter for the specified resource type.
//
// Azure specs typically express resource naming rules (pattern/minLength/maxLength) on the final path parameter of the PUT
// operation, e.g. /providers/Microsoft.Foo/widgets/{widgetName}.
//
// If no suitable parameter schema can be found, it returns (nil, nil).
func FindResourceNameSchema(doc *openapi3.T, resourceType string) (*openapi3.Schema, error) {
	if doc == nil || doc.Paths == nil {
		return nil, nil
	}

	// Normalize resource type for search (same rules as FindResource).
	searchType := resourceType
	if strings.HasSuffix(searchType, "}") {
		if idx := strings.LastIndex(searchType, "/{"); idx != -1 {
			searchType = searchType[:idx]
		}
	}

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil || pathItem.Put == nil {
			continue
		}

		parsedType, paramName, ok := azureARMInstancePathInfo(path)
		if !ok {
			continue
		}
		if !strings.EqualFold(parsedType, searchType) {
			continue
		}

		// Prefer operation-level parameters over path-level ones.
		if schema := findPathParameterSchema(pathItem.Put.Parameters, paramName); schema != nil {
			return normalizeStringSchemaForValidation(schema), nil
		}
		if schema := findPathParameterSchema(pathItem.Parameters, paramName); schema != nil {
			return normalizeStringSchemaForValidation(schema), nil
		}
	}

	return nil, nil
}

func findPathParameterSchema(params openapi3.Parameters, name string) *openapi3.Schema {
	for _, paramRef := range params {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		p := paramRef.Value
		if p.In != "path" {
			continue
		}
		if !strings.EqualFold(p.Name, name) {
			continue
		}

		// OpenAPI 3 parameters should have Schema, but many Azure specs are Swagger/OpenAPI 2.0
		// and express constraints (type/pattern/minLength/maxLength) at the parameter level.
		// kin-openapi preserves unknown fields under Extensions in that case.
		if p.Schema != nil && p.Schema.Value != nil {
			return p.Schema.Value
		}
		if schema := schemaFromParameterExtensions(p); schema != nil {
			return schema
		}
	}
	return nil
}

func schemaFromParameterExtensions(p *openapi3.Parameter) *openapi3.Schema {
	if p == nil {
		return nil
	}
	if p.Extensions == nil {
		return nil
	}

	getString := func(key string) (string, bool) {
		v, ok := p.Extensions[key]
		if !ok || v == nil {
			return "", false
		}
		s, ok := v.(string)
		return s, ok
	}
	getUint64 := func(key string) (*uint64, bool) {
		v, ok := p.Extensions[key]
		if !ok || v == nil {
			return nil, false
		}
		switch n := v.(type) {
		case float64:
			if n < 0 {
				return nil, false
			}
			u := uint64(n)
			return &u, true
		case int:
			if n < 0 {
				return nil, false
			}
			u := uint64(n)
			return &u, true
		case int64:
			if n < 0 {
				return nil, false
			}
			u := uint64(n)
			return &u, true
		case uint64:
			u := n
			return &u, true
		default:
			return nil, false
		}
	}

	typ, ok := getString("type")
	if !ok || typ != "string" {
		return nil
	}

	schema := &openapi3.Schema{Type: &openapi3.Types{"string"}}

	if pattern, ok := getString("pattern"); ok {
		schema.Pattern = pattern
	}
	if format, ok := getString("format"); ok {
		schema.Format = format
	}
	if min, ok := getUint64("minLength"); ok {
		schema.MinLength = *min
	}
	if max, ok := getUint64("maxLength"); ok {
		schema.MaxLength = max
	}

	// If no constraints were found, treat as absent.
	if schema.Pattern == "" && schema.MinLength == 0 && schema.MaxLength == nil && schema.Format == "" {
		return nil
	}

	return schema
}

// normalizeStringSchemaForValidation returns a schema suitable for string validation generation.
// Some Azure specs omit explicit "type: string" on parameters, but still provide string constraints.
func normalizeStringSchemaForValidation(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil {
		return nil
	}

	// If the schema is explicitly not a string, skip it.
	if schema.Type != nil {
		isString := false
		for _, t := range *schema.Type {
			if t == "string" {
				isString = true
				break
			}
		}
		if !isString {
			return nil
		}
	}

	// Copy only the fields we currently use for validation generation.
	copy := &openapi3.Schema{
		Type:      &openapi3.Types{"string"},
		Format:    schema.Format,
		Pattern:   schema.Pattern,
		MinLength: schema.MinLength,
		MaxLength: schema.MaxLength,
	}
	return copy
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
