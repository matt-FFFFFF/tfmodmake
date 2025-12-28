package openapi

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// GetEffectiveProperties returns the effective properties map for a schema,
// merging properties from allOf components if present.
// This is used for shape generation (types/locals) but preserves the original schema
// for validation generation which has different merge semantics.
func GetEffectiveProperties(schema *openapi3.Schema) (map[string]*openapi3.SchemaRef, error) {
	if schema == nil {
		return nil, nil
	}

	cache := make(map[*openapi3.Schema]map[string]*openapi3.SchemaRef)
	inProgress := make(map[*openapi3.Schema]struct{})
	return getEffectivePropertiesRecursive(schema, cache, inProgress)
}

func getEffectivePropertiesRecursive(schema *openapi3.Schema, cache map[*openapi3.Schema]map[string]*openapi3.SchemaRef, inProgress map[*openapi3.Schema]struct{}) (map[string]*openapi3.SchemaRef, error) {
	if schema == nil {
		return nil, nil
	}

	// Check cache first
	if cached, ok := cache[schema]; ok {
		return cached, nil
	}

	// Check for cycles
	if _, active := inProgress[schema]; active {
		return nil, fmt.Errorf("circular reference detected in allOf chain while getting effective properties")
	}

	// Mark as in-progress
	inProgress[schema] = struct{}{}
	defer delete(inProgress, schema)

	// If no allOf, return the schema's properties directly
	if len(schema.AllOf) == 0 {
		cache[schema] = schema.Properties
		return schema.Properties, nil
	}

	// Merge properties from allOf components
	result := make(map[string]*openapi3.SchemaRef)

	// Add base schema properties
	for name, propRef := range schema.Properties {
		result[name] = propRef
	}

	// Track which component defined each property for conflict detection
	propertyOrigins := make(map[string]int)
	for name := range schema.Properties {
		propertyOrigins[name] = -1 // base schema
	}

	// Merge from each allOf component
	for i, componentRef := range schema.AllOf {
		if componentRef == nil || componentRef.Value == nil {
			continue
		}

		// Recursively get effective properties for the component
		componentProps, err := getEffectivePropertiesRecursive(componentRef.Value, cache, inProgress)
		if err != nil {
			return nil, fmt.Errorf("getting properties from allOf component %d: %w", i, err)
		}

		for propName, propRef := range componentProps {
			if propRef == nil || propRef.Value == nil {
				continue
			}

			if existingRef, exists := result[propName]; exists {
				if existingRef != nil && existingRef.Value != nil {
					// Check if schemas are equivalent
					if !schemasEquivalent(existingRef.Value, propRef.Value) {
						originIdx := propertyOrigins[propName]
						originDesc := "base schema"
						if originIdx >= 0 {
							originDesc = fmt.Sprintf("allOf component %d", originIdx)
						}

						// Build detailed error message
						return nil, fmt.Errorf(
							"conflicting definitions for property %q in allOf:\n"+
								"  - %s defines it as type=%v, readOnly=%v, format=%q\n"+
								"  - allOf component %d defines it as type=%v, readOnly=%v, format=%q\n"+
								"Properties must have compatible schemas across allOf components",
							propName,
							originDesc, getSchemaType(existingRef.Value), existingRef.Value.ReadOnly, existingRef.Value.Format,
							i, getSchemaType(propRef.Value), propRef.Value.ReadOnly, propRef.Value.Format,
						)
					}
				}
			} else {
				result[propName] = propRef
				propertyOrigins[propName] = i
			}
		}
	}

	cache[schema] = result
	return result, nil
}

// GetEffectiveRequired returns the effective required fields list for a schema,
// merging required arrays from allOf components if present (union semantics).
func GetEffectiveRequired(schema *openapi3.Schema) ([]string, error) {
	if schema == nil {
		return nil, nil
	}

	cache := make(map[*openapi3.Schema][]string)
	inProgress := make(map[*openapi3.Schema]struct{})
	return getEffectiveRequiredRecursive(schema, cache, inProgress)
}

func getEffectiveRequiredRecursive(schema *openapi3.Schema, cache map[*openapi3.Schema][]string, inProgress map[*openapi3.Schema]struct{}) ([]string, error) {
	if schema == nil {
		return nil, nil
	}

	// Check cache first
	if cached, ok := cache[schema]; ok {
		return cached, nil
	}

	// Check for cycles
	if _, active := inProgress[schema]; active {
		return nil, fmt.Errorf("circular reference detected in allOf chain while getting effective required fields")
	}

	// Mark as in-progress
	inProgress[schema] = struct{}{}
	defer delete(inProgress, schema)

	// If no allOf, return the schema's required directly
	if len(schema.AllOf) == 0 {
		result := make([]string, len(schema.Required))
		copy(result, schema.Required)
		cache[schema] = result
		return result, nil
	}

	// Union required fields from all components
	requiredSet := make(map[string]struct{})

	// Add base schema required fields
	for _, req := range schema.Required {
		requiredSet[req] = struct{}{}
	}

	// Add from each allOf component
	for i, componentRef := range schema.AllOf {
		if componentRef == nil || componentRef.Value == nil {
			continue
		}

		// Recursively get effective required for the component
		componentRequired, err := getEffectiveRequiredRecursive(componentRef.Value, cache, inProgress)
		if err != nil {
			return nil, fmt.Errorf("getting required from allOf component %d: %w", i, err)
		}

		for _, req := range componentRequired {
			requiredSet[req] = struct{}{}
		}
	}

	// Convert set to sorted slice
	result := make([]string, 0, len(requiredSet))
	for req := range requiredSet {
		result = append(result, req)
	}
	sort.Strings(result)

	cache[schema] = result
	return result, nil
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
