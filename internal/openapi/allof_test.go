package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenAllOf_SimpleComposition(t *testing.T) {
	t.Parallel()

	// Base schema with one property
	base := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"name": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
		},
		Required: []string{"name"},
	}

	// Extension schema with additional property
	extension := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"age": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"integer"},
				},
			},
		},
		Required: []string{"age"},
	}

	// Schema with allOf
	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: base},
			{Value: extension},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Check that both properties are present
	assert.Contains(t, flattened.Properties, "name")
	assert.Contains(t, flattened.Properties, "age")
	assert.Len(t, flattened.Properties, 2)

	// Check that both required fields are present
	assert.Contains(t, flattened.Required, "name")
	assert.Contains(t, flattened.Required, "age")
	assert.Len(t, flattened.Required, 2)
}

func TestFlattenAllOf_RequiredFieldMerging(t *testing.T) {
	t.Parallel()

	component1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			"field2": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
		Required: []string{"field1"},
	}

	component2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field3": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
		Required: []string{"field2", "field3"},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: component1},
			{Value: component2},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// All three properties should be present
	assert.Len(t, flattened.Properties, 3)

	// All three fields should be required (union of required arrays)
	assert.ElementsMatch(t, []string{"field1", "field2", "field3"}, flattened.Required)
}

func TestFlattenAllOf_ReadOnlyFieldExclusion(t *testing.T) {
	t.Parallel()

	component1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"id": {
				Value: &openapi3.Schema{
					Type:     &openapi3.Types{"string"},
					ReadOnly: true,
				},
			},
			"name": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
		},
		Required: []string{"id", "name"},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: component1},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Both properties should be present
	assert.Len(t, flattened.Properties, 2)
	assert.Contains(t, flattened.Properties, "id")
	assert.Contains(t, flattened.Properties, "name")

	// Both are marked as required in the schema
	assert.Contains(t, flattened.Required, "id")
	assert.Contains(t, flattened.Required, "name")

	// But the readOnly field should have ReadOnly=true
	assert.True(t, flattened.Properties["id"].Value.ReadOnly)
	assert.False(t, flattened.Properties["name"].Value.ReadOnly)
}

func TestFlattenAllOf_EquivalentPropertiesAllowed(t *testing.T) {
	t.Parallel()

	// Both components define the same property with identical schemas
	component1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"status": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					Description: "The status of the resource",
				},
			},
		},
	}

	component2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"status": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					Description: "Status of the resource (different wording)", // Different description is OK
				},
			},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: component1},
			{Value: component2},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Should succeed because schemas are equivalent (ignoring description differences)
	assert.Contains(t, flattened.Properties, "status")
}

func TestFlattenAllOf_ConflictingPropertiesError(t *testing.T) {
	t.Parallel()

	// Components define the same property with incompatible types
	component1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"count": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"integer"},
					Description: "Count as integer",
				},
			},
		},
	}

	component2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"count": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					Description: "Count as string",
				},
			},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: component1},
			{Value: component2},
		},
	}

	flattened, err := FlattenAllOf(schema)
	assert.Error(t, err)
	assert.Nil(t, flattened)
	assert.Contains(t, err.Error(), "conflicting definitions for property \"count\"")
	assert.Contains(t, err.Error(), "component 1")
}

func TestFlattenAllOf_NestedAllOf(t *testing.T) {
	t.Parallel()

	// Inner allOf
	innerComponent1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"inner1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	innerComponent2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"inner2": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	innerAllOf := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: innerComponent1},
			{Value: innerComponent2},
		},
	}

	// Outer allOf
	outerComponent := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"outer": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: innerAllOf},
			{Value: outerComponent},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// All properties from nested allOf should be present
	assert.Contains(t, flattened.Properties, "inner1")
	assert.Contains(t, flattened.Properties, "inner2")
	assert.Contains(t, flattened.Properties, "outer")
	assert.Len(t, flattened.Properties, 3)
}

func TestFlattenAllOf_RecursivePropertyFlattening(t *testing.T) {
	t.Parallel()

	// Create nested property with allOf
	nestedAllOf := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"nestedField1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
			{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"nestedField2": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
					},
				},
			},
		},
	}

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"nested": {Value: nestedAllOf},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Check that nested property was flattened
	assert.Contains(t, flattened.Properties, "nested")
	nestedProp := flattened.Properties["nested"].Value
	assert.NotNil(t, nestedProp)
	assert.Contains(t, nestedProp.Properties, "nestedField1")
	assert.Contains(t, nestedProp.Properties, "nestedField2")
}

func TestFlattenAllOf_NoAllOf(t *testing.T) {
	t.Parallel()

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
		Required: []string{"field"},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Should return the same schema (possibly with recursive processing)
	assert.Contains(t, flattened.Properties, "field")
	assert.Contains(t, flattened.Required, "field")
}

func TestFlattenAllOf_NilSchema(t *testing.T) {
	t.Parallel()

	flattened, err := FlattenAllOf(nil)
	assert.NoError(t, err)
	assert.Nil(t, flattened)
}

func TestFlattenAllOf_CycleDetection(t *testing.T) {
	t.Parallel()

	// Create a self-referential schema (error details containing error details)
	// This is valid in OpenAPI and should be handled gracefully
	errorDetails := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"code":    {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			"message": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			"details": {Value: nil}, // Will point to array of errorDetails
		},
	}

	// Create the self-referential array
	errorDetails.Properties["details"].Value = &openapi3.Schema{
		Type: &openapi3.Types{"array"},
		Items: &openapi3.SchemaRef{
			Value: errorDetails, // Points back to itself
		},
	}

	flattened, err := FlattenAllOf(errorDetails)
	// Should handle the cycle gracefully via caching
	assert.NoError(t, err)
	assert.NotNil(t, flattened)
	assert.Contains(t, flattened.Properties, "code")
	assert.Contains(t, flattened.Properties, "message")
	assert.Contains(t, flattened.Properties, "details")
}

func TestFlattenAllOf_ArrayItemsWithAllOf(t *testing.T) {
	t.Parallel()

	// Array items with allOf
	itemsSchema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"prop1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
			{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"prop2": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
					},
				},
			},
		},
	}

	schema := &openapi3.Schema{
		Type:  &openapi3.Types{"array"},
		Items: &openapi3.SchemaRef{Value: itemsSchema},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Check that array items were flattened
	assert.NotNil(t, flattened.Items)
	assert.NotNil(t, flattened.Items.Value)
	assert.Contains(t, flattened.Items.Value.Properties, "prop1")
	assert.Contains(t, flattened.Items.Value.Properties, "prop2")
}

func TestFlattenAllOf_ExtensionMerging(t *testing.T) {
	t.Parallel()

	component1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Extensions: map[string]any{
			"x-ms-mutability": []string{"read", "create"},
		},
		Properties: map[string]*openapi3.SchemaRef{
			"field1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	component2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Extensions: map[string]any{
			"x-custom": "value",
		},
		Properties: map[string]*openapi3.SchemaRef{
			"field2": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: component1},
			{Value: component2},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// Both extensions should be present
	assert.Contains(t, flattened.Extensions, "x-ms-mutability")
	assert.Contains(t, flattened.Extensions, "x-custom")
}

func TestFlattenAllOf_ComplexRealWorldExample(t *testing.T) {
	t.Parallel()

	// Simulate Azure common types Resource pattern
	azureResource := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"id": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					ReadOnly:    true,
					Description: "Resource ID",
				},
			},
			"name": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					ReadOnly:    true,
					Description: "Resource name",
				},
			},
			"type": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					ReadOnly:    true,
					Description: "Resource type",
				},
			},
		},
		Required: []string{"id", "name", "type"},
	}

	// Specific resource with additional properties
	managedCluster := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"kubernetesVersion": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "Kubernetes version",
							},
						},
						"dnsPrefix": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "DNS prefix",
							},
						},
					},
					Required: []string{"kubernetesVersion"},
				},
			},
			"location": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					Description: "Resource location",
				},
			},
		},
		Required: []string{"location", "properties"},
	}

	// Combine with allOf
	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: azureResource},
			{Value: managedCluster},
		},
	}

	flattened, err := FlattenAllOf(schema)
	require.NoError(t, err)
	require.NotNil(t, flattened)

	// All top-level properties should be present
	assert.Contains(t, flattened.Properties, "id")
	assert.Contains(t, flattened.Properties, "name")
	assert.Contains(t, flattened.Properties, "type")
	assert.Contains(t, flattened.Properties, "properties")
	assert.Contains(t, flattened.Properties, "location")

	// ReadOnly fields should still be readOnly
	assert.True(t, flattened.Properties["id"].Value.ReadOnly)
	assert.True(t, flattened.Properties["name"].Value.ReadOnly)
	assert.True(t, flattened.Properties["type"].Value.ReadOnly)
	assert.False(t, flattened.Properties["location"].Value.ReadOnly)

	// Required fields should be unioned
	assert.ElementsMatch(t, []string{"id", "name", "type", "location", "properties"}, flattened.Required)

	// Nested properties should be accessible
	props := flattened.Properties["properties"].Value
	assert.NotNil(t, props)
	assert.Contains(t, props.Properties, "kubernetesVersion")
	assert.Contains(t, props.Properties, "dnsPrefix")
	assert.Contains(t, props.Required, "kubernetesVersion")
}
