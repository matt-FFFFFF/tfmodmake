package openapi

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// FlattenAllOf merges allOf components into a single effective schema for generation.
// It handles property merging, required field combination, and conflict detection.
func FlattenAllOf(schema *openapi3.Schema) (*openapi3.Schema, error) {
	if schema == nil {
		return nil, nil
	}

	// Use a cache to avoid reprocessing the same schema and handle cycles
	cache := make(map[*openapi3.Schema]*openapi3.Schema)
	return flattenAllOfRecursive(schema, cache)
}

func flattenAllOfRecursive(schema *openapi3.Schema, cache map[*openapi3.Schema]*openapi3.Schema) (*openapi3.Schema, error) {
	if schema == nil {
		return nil, nil
	}

	// Check cache first - if we've already processed this schema, return the cached result
	if cached, ok := cache[schema]; ok {
		return cached, nil
	}

	// For schemas without allOf, we still want to process nested properties once
	// but we need to handle recursion. We'll mark the schema as "in progress" by
	// caching it as itself first
	if len(schema.AllOf) == 0 {
		cache[schema] = schema // Mark as being processed

		// Process nested properties
		if schema.Properties != nil {
			for propName, propRef := range schema.Properties {
				if propRef != nil && propRef.Value != nil {
					// Skip if this property points back to a schema we're already processing
					if _, inProgress := cache[propRef.Value]; inProgress && propRef.Value == schema {
						continue
					}
					flattened, err := flattenAllOfRecursive(propRef.Value, cache)
					if err != nil {
						return nil, fmt.Errorf("flattening property %s: %w", propName, err)
					}
					propRef.Value = flattened
				}
			}
		}

		// Process array items
		if schema.Items != nil && schema.Items.Value != nil {
			if _, inProgress := cache[schema.Items.Value]; !inProgress || schema.Items.Value != schema {
				flattened, err := flattenAllOfRecursive(schema.Items.Value, cache)
				if err != nil {
					return nil, fmt.Errorf("flattening array items: %w", err)
				}
				schema.Items.Value = flattened
			}
		}

		// Process additional properties
		if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
			if _, inProgress := cache[schema.AdditionalProperties.Schema.Value]; !inProgress || schema.AdditionalProperties.Schema.Value != schema {
				flattened, err := flattenAllOfRecursive(schema.AdditionalProperties.Schema.Value, cache)
				if err != nil {
					return nil, fmt.Errorf("flattening additional properties: %w", err)
				}
				schema.AdditionalProperties.Schema.Value = flattened
			}
		}

		return schema, nil
	}

	// Merge allOf components
	merged := &openapi3.Schema{
		Properties: make(map[string]*openapi3.SchemaRef),
		Required:   make([]string, 0),
	}

	// Copy base schema properties
	if schema.Type != nil {
		merged.Type = schema.Type
	}
	if schema.Description != "" {
		merged.Description = schema.Description
	}
	if schema.ReadOnly {
		merged.ReadOnly = schema.ReadOnly
	}
	if schema.WriteOnly {
		merged.WriteOnly = schema.WriteOnly
	}
	if len(schema.Enum) > 0 {
		merged.Enum = schema.Enum
	}
	if schema.MinLength > 0 {
		merged.MinLength = schema.MinLength
	}
	if schema.MaxLength != nil {
		merged.MaxLength = schema.MaxLength
	}
	if schema.Min != nil {
		merged.Min = schema.Min
	}
	if schema.Max != nil {
		merged.Max = schema.Max
	}
	if schema.ExclusiveMin {
		merged.ExclusiveMin = schema.ExclusiveMin
	}
	if schema.ExclusiveMax {
		merged.ExclusiveMax = schema.ExclusiveMax
	}
	if schema.MultipleOf != nil {
		merged.MultipleOf = schema.MultipleOf
	}
	if schema.MinItems > 0 {
		merged.MinItems = schema.MinItems
	}
	if schema.MaxItems != nil {
		merged.MaxItems = schema.MaxItems
	}
	if schema.UniqueItems {
		merged.UniqueItems = schema.UniqueItems
	}
	if schema.Format != "" {
		merged.Format = schema.Format
	}
	if schema.Items != nil {
		merged.Items = schema.Items
	}
	if schema.AdditionalProperties.Has != nil {
		merged.AdditionalProperties.Has = schema.AdditionalProperties.Has
	}
	if schema.AdditionalProperties.Schema != nil {
		merged.AdditionalProperties.Schema = schema.AdditionalProperties.Schema
	}
	if schema.Extensions != nil {
		merged.Extensions = make(map[string]any)
		for k, v := range schema.Extensions {
			merged.Extensions[k] = v
		}
	}

	// Add base schema properties
	for name, propRef := range schema.Properties {
		merged.Properties[name] = propRef
	}

	// Add base required fields
	merged.Required = append(merged.Required, schema.Required...)

	// Track property definitions for conflict detection
	propertyOrigins := make(map[string]*openapi3.Schema)
	for name := range schema.Properties {
		propertyOrigins[name] = schema
	}

	// Merge each allOf component
	for i, componentRef := range schema.AllOf {
		if componentRef == nil || componentRef.Value == nil {
			continue
		}

		// Recursively flatten the component
		component, err := flattenAllOfRecursive(componentRef.Value, cache)
		if err != nil {
			return nil, fmt.Errorf("flattening allOf component %d: %w", i, err)
		}

		// Merge properties
		for propName, propRef := range component.Properties {
			if propRef == nil || propRef.Value == nil {
				continue
			}

			if existingRef, exists := merged.Properties[propName]; exists {
				if existingRef != nil && existingRef.Value != nil {
					// Check if schemas are equivalent
					if !schemasEquivalent(existingRef.Value, propRef.Value) {
						originSchema := propertyOrigins[propName]
						return nil, fmt.Errorf(
							"conflicting definitions for property %q in allOf: "+
								"component %d defines it differently than previous definition. "+
								"First defined in schema with type=%v, description=%q; "+
								"conflicting definition has type=%v, description=%q",
							propName, i,
							getSchemaType(existingRef.Value), getDescription(originSchema, propName),
							getSchemaType(propRef.Value), propRef.Value.Description,
						)
					}
				}
			} else {
				merged.Properties[propName] = propRef
				propertyOrigins[propName] = component
			}
		}

		// Union required fields
		for _, req := range component.Required {
			if !contains(merged.Required, req) {
				merged.Required = append(merged.Required, req)
			}
		}

		// Merge type if not set
		if merged.Type == nil && component.Type != nil {
			merged.Type = component.Type
		}

		// Merge description if not set
		if merged.Description == "" && component.Description != "" {
			merged.Description = component.Description
		}

		// ReadOnly from any component makes the merged schema readOnly
		if component.ReadOnly {
			merged.ReadOnly = true
		}

		// WriteOnly from any component makes the merged schema writeOnly
		if component.WriteOnly {
			merged.WriteOnly = true
		}

		// Merge extensions
		if component.Extensions != nil {
			if merged.Extensions == nil {
				merged.Extensions = make(map[string]any)
			}
			for k, v := range component.Extensions {
				if _, exists := merged.Extensions[k]; !exists {
					merged.Extensions[k] = v
				}
			}
		}
	}

	// Sort required for consistency
	sort.Strings(merged.Required)

	// Cache the merged result before processing nested properties
	cache[schema] = merged

	// Recursively process merged properties
	for propName, propRef := range merged.Properties {
		if propRef != nil && propRef.Value != nil {
			flattened, err := flattenAllOfRecursive(propRef.Value, cache)
			if err != nil {
				return nil, fmt.Errorf("flattening merged property %s: %w", propName, err)
			}
			propRef.Value = flattened
		}
	}

	// Process array items if present
	if merged.Items != nil && merged.Items.Value != nil {
		flattened, err := flattenAllOfRecursive(merged.Items.Value, cache)
		if err != nil {
			return nil, fmt.Errorf("flattening merged array items: %w", err)
		}
		merged.Items.Value = flattened
	}

	// Process additional properties if present
	if merged.AdditionalProperties.Schema != nil && merged.AdditionalProperties.Schema.Value != nil {
		flattened, err := flattenAllOfRecursive(merged.AdditionalProperties.Schema.Value, cache)
		if err != nil {
			return nil, fmt.Errorf("flattening merged additional properties: %w", err)
		}
		merged.AdditionalProperties.Schema.Value = flattened
	}

	return merged, nil
}

// schemasEquivalent checks if two schemas are equivalent for the purposes of allOf merging.
// It's tolerant of differences in documentation and extension fields.
func schemasEquivalent(a, b *openapi3.Schema) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare types
	if !typesEqual(a.Type, b.Type) {
		return false
	}

	// Compare readOnly/writeOnly
	if a.ReadOnly != b.ReadOnly || a.WriteOnly != b.WriteOnly {
		return false
	}

	// Compare format
	if a.Format != b.Format {
		return false
	}

	// Compare enum values
	if !enumsEqual(a.Enum, b.Enum) {
		return false
	}

	// Compare constraints
	if a.MinLength != b.MinLength {
		return false
	}
	if !uint64PtrEqual(a.MaxLength, b.MaxLength) {
		return false
	}
	if !float64PtrEqual(a.Min, b.Min) {
		return false
	}
	if !float64PtrEqual(a.Max, b.Max) {
		return false
	}
	if a.ExclusiveMin != b.ExclusiveMin || a.ExclusiveMax != b.ExclusiveMax {
		return false
	}
	if !float64PtrEqual(a.MultipleOf, b.MultipleOf) {
		return false
	}
	if a.MinItems != b.MinItems {
		return false
	}
	if !uint64PtrEqual(a.MaxItems, b.MaxItems) {
		return false
	}
	if a.UniqueItems != b.UniqueItems {
		return false
	}

	// For objects, compare properties recursively
	if isObjectType(a) || isObjectType(b) {
		if len(a.Properties) != len(b.Properties) {
			return false
		}
		for name, aProp := range a.Properties {
			bProp, exists := b.Properties[name]
			if !exists {
				return false
			}
			if aProp == nil && bProp == nil {
				continue
			}
			if aProp == nil || bProp == nil {
				return false
			}
			if !schemasEquivalent(aProp.Value, bProp.Value) {
				return false
			}
		}

		// Compare required fields
		aReq := make([]string, len(a.Required))
		copy(aReq, a.Required)
		sort.Strings(aReq)
		bReq := make([]string, len(b.Required))
		copy(bReq, b.Required)
		sort.Strings(bReq)
		if !reflect.DeepEqual(aReq, bReq) {
			return false
		}
	}

	// For arrays, compare items
	if isArrayType(a) || isArrayType(b) {
		if a.Items == nil && b.Items == nil {
			return true
		}
		if a.Items == nil || b.Items == nil {
			return false
		}
		if !schemasEquivalent(a.Items.Value, b.Items.Value) {
			return false
		}
	}

	return true
}

func isObjectType(s *openapi3.Schema) bool {
	if s == nil || s.Type == nil {
		return false
	}
	for _, t := range *s.Type {
		if t == "object" {
			return true
		}
	}
	return false
}

func isArrayType(s *openapi3.Schema) bool {
	if s == nil || s.Type == nil {
		return false
	}
	for _, t := range *s.Type {
		if t == "array" {
			return true
		}
	}
	return false
}

func typesEqual(a, b *openapi3.Types) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	aTypes := *a
	bTypes := *b
	if len(aTypes) != len(bTypes) {
		return false
	}
	aCopy := make([]string, len(aTypes))
	copy(aCopy, aTypes)
	sort.Strings(aCopy)
	bCopy := make([]string, len(bTypes))
	copy(bCopy, bTypes)
	sort.Strings(bCopy)
	return reflect.DeepEqual(aCopy, bCopy)
}

func enumsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	// Convert to string representations for comparison
	aStrs := make([]string, len(a))
	for i, v := range a {
		aStrs[i] = fmt.Sprintf("%v", v)
	}
	sort.Strings(aStrs)

	bStrs := make([]string, len(b))
	for i, v := range b {
		bStrs[i] = fmt.Sprintf("%v", v)
	}
	sort.Strings(bStrs)

	return reflect.DeepEqual(aStrs, bStrs)
}

func uint64PtrEqual(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func float64PtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getSchemaType(s *openapi3.Schema) string {
	if s == nil || s.Type == nil {
		return "unknown"
	}
	types := *s.Type
	if len(types) == 0 {
		return "unknown"
	}
	return types[0]
}

func getDescription(schema *openapi3.Schema, propName string) string {
	if schema == nil {
		return ""
	}
	if propRef, ok := schema.Properties[propName]; ok && propRef != nil && propRef.Value != nil {
		return propRef.Value.Description
	}
	return ""
}
