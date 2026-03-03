package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitive(t *testing.T) {
	t.Run("nil property returns false", func(t *testing.T) {
		assert.False(t, IsSensitive(nil))
	})

	t.Run("non-sensitive property returns false", func(t *testing.T) {
		prop := &Property{
			Name:      "name",
			Type:      TypeString,
			Sensitive: false,
		}
		assert.False(t, IsSensitive(prop))
	})

	t.Run("sensitive property returns true", func(t *testing.T) {
		prop := &Property{
			Name:      "password",
			Type:      TypeString,
			Sensitive: true,
		}
		assert.True(t, IsSensitive(prop))
	})
}

func TestCollectSecretFields(t *testing.T) {
	t.Run("nil schema returns nil", func(t *testing.T) {
		result := CollectSecretFields(nil)
		assert.Nil(t, result)
	})

	t.Run("no secrets returns empty slice", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"name": {
					Name: "name",
					Type: TypeString,
				},
				"location": {
					Name: "location",
					Type: TypeString,
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Empty(t, result)
	})

	t.Run("top-level sensitive field", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"name": {
					Name: "name",
					Type: TypeString,
				},
				"apiKey": {
					Name:      "apiKey",
					Type:      TypeString,
					Sensitive: true,
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Len(t, result, 1)
		assert.Equal(t, "apiKey", result[0].Path)
		assert.Equal(t, "apiKey", result[0].Property.Name)
	})

	t.Run("nested sensitive field", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"properties": {
					Name: "properties",
					Type: TypeObject,
					Children: map[string]*Property{
						"adminPassword": {
							Name:      "adminPassword",
							Type:      TypeString,
							Sensitive: true,
						},
						"adminUsername": {
							Name: "adminUsername",
							Type: TypeString,
						},
					},
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Len(t, result, 1)
		assert.Equal(t, "properties.adminPassword", result[0].Path)
	})

	t.Run("deeply nested sensitive fields", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"properties": {
					Name: "properties",
					Type: TypeObject,
					Children: map[string]*Property{
						"config": {
							Name: "config",
							Type: TypeObject,
							Children: map[string]*Property{
								"connectionString": {
									Name:      "connectionString",
									Type:      TypeString,
									Sensitive: true,
								},
								"key": {
									Name:      "key",
									Type:      TypeString,
									Sensitive: true,
								},
							},
						},
					},
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Len(t, result, 2)

		paths := make(map[string]bool)
		for _, sf := range result {
			paths[sf.Path] = true
		}
		assert.True(t, paths["properties.config.connectionString"])
		assert.True(t, paths["properties.config.key"])
	})

	t.Run("sensitive field in array item type", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"items": {
					Name: "items",
					Type: TypeArray,
					ItemType: &Property{
						Name: "item",
						Type: TypeObject,
						Children: map[string]*Property{
							"secret": {
								Name:      "secret",
								Type:      TypeString,
								Sensitive: true,
							},
						},
					},
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Len(t, result, 1)
		assert.Equal(t, "items[].secret", result[0].Path)
	})

	t.Run("multiple sensitive fields at different levels", func(t *testing.T) {
		schema := &ResourceSchema{
			Properties: map[string]*Property{
				"topSecret": {
					Name:      "topSecret",
					Type:      TypeString,
					Sensitive: true,
				},
				"nested": {
					Name: "nested",
					Type: TypeObject,
					Children: map[string]*Property{
						"innerSecret": {
							Name:      "innerSecret",
							Type:      TypeString,
							Sensitive: true,
						},
					},
				},
			},
		}
		result := CollectSecretFields(schema)
		assert.Len(t, result, 2)

		paths := make(map[string]bool)
		for _, sf := range result {
			paths[sf.Path] = true
		}
		assert.True(t, paths["topSecret"])
		assert.True(t, paths["nested.innerSecret"])
	})
}
