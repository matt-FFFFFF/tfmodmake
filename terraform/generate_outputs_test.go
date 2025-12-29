package terraform

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func TestOutputNameForExportPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "empty", path: "", expected: ""},
		{name: "properties prefix stripped", path: "properties.foo", expected: "foo"},
		{name: "nested properties", path: "properties.foo.bar", expected: "foo_bar"},
		{name: "reserved name (name)", path: "name", expected: ""},
		{name: "reserved name (resource_id)", path: "resource_id", expected: ""},
		{name: "reserved name (id)", path: "id", expected: ""},
		{name: "identity nested", path: "identity.principalId", expected: "identity_principal_id"},
		{name: "trims whitespace", path: "  properties.foo  ", expected: "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, outputNameForExportPath(tt.path))
		})
	}
}

func TestDefaultTokensForSchema(t *testing.T) {
	assert.Equal(t, "null", string(defaultTokensForSchema(nil).Bytes()))

	obj := &openapi3.Schema{Type: &openapi3.Types{"object"}}
	assert.Equal(t, "{}", string(defaultTokensForSchema(obj).Bytes()))

	arr := &openapi3.Schema{Type: &openapi3.Types{"array"}}
	assert.Equal(t, "[]", string(defaultTokensForSchema(arr).Bytes()))

	scalar := &openapi3.Schema{Type: &openapi3.Types{"string"}}
	assert.Equal(t, "null", string(defaultTokensForSchema(scalar).Bytes()))
}

func TestSchemaForExportPath_FindsInAllOf(t *testing.T) {
	base := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"foo": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Description: "Foo description"}},
		},
	}
	root := &openapi3.Schema{
		Type:  &openapi3.Types{"object"},
		AllOf: openapi3.SchemaRefs{{Value: base}},
	}

	got := schemaForExportPath(root, "foo")
	if assert.NotNil(t, got) {
		assert.Equal(t, "Foo description", got.Description)
	}
}

func TestSchemaForExportPath_NestedProperties(t *testing.T) {
	nested := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"bar": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Description: "Bar description"}},
		},
	}
	root := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {Value: nested},
		},
	}

	got := schemaForExportPath(root, "properties.bar")
	if assert.NotNil(t, got) {
		assert.Equal(t, "Bar description", got.Description)
	}
}

func TestSchemaForExportPath_CircularDoesNotLoop(t *testing.T) {
	s := &openapi3.Schema{Type: &openapi3.Types{"object"}}
	s.AllOf = openapi3.SchemaRefs{{Value: s}}

	assert.NotPanics(t, func() {
		got := schemaForExportPath(s, "foo")
		assert.Nil(t, got)
	})
}
