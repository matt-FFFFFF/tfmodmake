package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEffectiveProperties_SimpleAllOf(t *testing.T) {
	t.Parallel()

	base := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"name": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	extension := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"age": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: base},
			{Value: extension},
		},
	}

	props, err := GetEffectiveProperties(schema)
	require.NoError(t, err)
	require.NotNil(t, props)
	assert.Len(t, props, 2)
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "age")
}

func TestGetEffectiveProperties_NoAllOf(t *testing.T) {
	t.Parallel()

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	props, err := GetEffectiveProperties(schema)
	require.NoError(t, err)
	// Should return the same properties map
	assert.Len(t, props, 1)
	assert.Contains(t, props, "field")
}

func TestGetEffectiveProperties_Conflict(t *testing.T) {
	t.Parallel()

	comp1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"count": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
		},
	}

	comp2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"count": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: comp1},
			{Value: comp2},
		},
	}

	props, err := GetEffectiveProperties(schema)
	assert.Error(t, err)
	assert.Nil(t, props)
	assert.Contains(t, err.Error(), "conflicting definitions for property \"count\"")
}

func TestGetEffectiveProperties_CycleDetection(t *testing.T) {
	t.Parallel()

	// Create A→B→A cycle
	schemaA := &openapi3.Schema{
		Type:   &openapi3.Types{"object"},
		AllOf:  []*openapi3.SchemaRef{},
	}

	schemaB := &openapi3.Schema{
		Type:   &openapi3.Types{"object"},
		AllOf:  []*openapi3.SchemaRef{{Value: schemaA}},
	}

	schemaA.AllOf = []*openapi3.SchemaRef{{Value: schemaB}}

	props, err := GetEffectiveProperties(schemaA)
	assert.Error(t, err)
	assert.Nil(t, props)
	assert.Contains(t, err.Error(), "circular reference")
}

func TestGetEffectiveProperties_NestedAllOf(t *testing.T) {
	t.Parallel()

	inner1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"inner1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	inner2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"inner2": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	innerAllOf := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: inner1},
			{Value: inner2},
		},
	}

	outer := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"outer": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: innerAllOf},
			{Value: outer},
		},
	}

	props, err := GetEffectiveProperties(schema)
	require.NoError(t, err)
	assert.Len(t, props, 3)
	assert.Contains(t, props, "inner1")
	assert.Contains(t, props, "inner2")
	assert.Contains(t, props, "outer")
}

func TestGetEffectiveRequired_SimpleUnion(t *testing.T) {
	t.Parallel()

	comp1 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field1"},
	}

	comp2 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field2", "field3"},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: comp1},
			{Value: comp2},
		},
	}

	required, err := GetEffectiveRequired(schema)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"field1", "field2", "field3"}, required)
}

func TestGetEffectiveRequired_NoAllOf(t *testing.T) {
	t.Parallel()

	schema := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field1", "field2"},
	}

	required, err := GetEffectiveRequired(schema)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"field1", "field2"}, required)
}

func TestGetEffectiveRequired_CycleDetection(t *testing.T) {
	t.Parallel()

	// Create A→B→A cycle
	schemaA := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field1"},
		AllOf:    []*openapi3.SchemaRef{},
	}

	schemaB := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field2"},
		AllOf:    []*openapi3.SchemaRef{{Value: schemaA}},
	}

	schemaA.AllOf = []*openapi3.SchemaRef{{Value: schemaB}}

	required, err := GetEffectiveRequired(schemaA)
	assert.Error(t, err)
	assert.Nil(t, required)
	assert.Contains(t, err.Error(), "circular reference")
}

func TestGetEffectiveRequired_DuplicatesHandled(t *testing.T) {
	t.Parallel()

	comp1 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field1", "field2"},
	}

	comp2 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"field2", "field3"},
	}

	schema := &openapi3.Schema{
		Required: []string{"field1"},
		AllOf: []*openapi3.SchemaRef{
			{Value: comp1},
			{Value: comp2},
		},
	}

	required, err := GetEffectiveRequired(schema)
	require.NoError(t, err)
	// Should have union with no duplicates
	assert.ElementsMatch(t, []string{"field1", "field2", "field3"}, required)
	assert.Len(t, required, 3) // Ensure no duplicates
}

func TestGetEffectiveRequired_NestedAllOf(t *testing.T) {
	t.Parallel()

	inner1 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"inner1"},
	}

	inner2 := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"inner2"},
	}

	innerAllOf := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: inner1},
			{Value: inner2},
		},
	}

	outer := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"outer"},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: innerAllOf},
			{Value: outer},
		},
	}

	required, err := GetEffectiveRequired(schema)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"inner1", "inner2", "outer"}, required)
}

func TestGetEffectiveProperties_Memoization(t *testing.T) {
	t.Parallel()

	// Create a schema that's referenced multiple times
	shared := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"shared": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	comp1 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
		AllOf: []*openapi3.SchemaRef{{Value: shared}},
	}

	comp2 := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"field2": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
		AllOf: []*openapi3.SchemaRef{{Value: shared}},
	}

	schema := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: comp1},
			{Value: comp2},
		},
	}

	// Should not error due to memoization preventing double-processing
	props, err := GetEffectiveProperties(schema)
	require.NoError(t, err)
	assert.Contains(t, props, "field1")
	assert.Contains(t, props, "field2")
	assert.Contains(t, props, "shared")
}
