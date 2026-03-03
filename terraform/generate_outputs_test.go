package terraform

import (
	"testing"

	"github.com/matt-FFFFFF/tfmodmake/schema"
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

func TestDefaultTokensForProperty(t *testing.T) {
	assert.Equal(t, "null", string(defaultTokensForProperty(nil).Bytes()))

	obj := &schema.Property{Type: schema.TypeObject}
	assert.Equal(t, "{}", string(defaultTokensForProperty(obj).Bytes()))

	arr := &schema.Property{Type: schema.TypeArray}
	assert.Equal(t, "[]", string(defaultTokensForProperty(arr).Bytes()))

	scalar := &schema.Property{Type: schema.TypeString}
	assert.Equal(t, "null", string(defaultTokensForProperty(scalar).Bytes()))
}

func TestPropertyForExportPath_FindsNestedProperty(t *testing.T) {
	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"foo": {Name: "foo", Type: schema.TypeString, Description: "Foo description"},
		},
	}

	got := propertyForExportPath(rs, "foo")
	if assert.NotNil(t, got) {
		assert.Equal(t, "Foo description", got.Description)
	}
}

func TestPropertyForExportPath_NestedProperties(t *testing.T) {
	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"bar": {Name: "bar", Type: schema.TypeString, Description: "Bar description"},
			}},
		},
	}

	got := propertyForExportPath(rs, "properties.bar")
	if assert.NotNil(t, got) {
		assert.Equal(t, "Bar description", got.Description)
	}
}
