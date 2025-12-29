package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindResource(t *testing.T) {
	tests := []struct {
		name         string
		doc          *openapi3.T
		resourceType string
		wantSchema   bool
		wantErr      bool
	}{
		{
			name: "Resource found",
			doc: &openapi3.T{
				Paths: &openapi3.Paths{
					Extensions: map[string]interface{}{},
				},
			},
			resourceType: "Microsoft.ContainerService/managedClusters",
			wantSchema:   true,
			wantErr:      false,
		},
		{
			name: "Resource not found",
			doc: &openapi3.T{
				Paths: &openapi3.Paths{
					Extensions: map[string]interface{}{},
				},
			},
			resourceType: "Microsoft.Compute/virtualMachines",
			wantSchema:   false,
			wantErr:      true,
		},
	}

	// Setup doc for "Resource found" case
	pathItem := &openapi3.PathItem{
		Put: &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
								},
							},
						},
					},
				},
			},
		},
	}
	tests[0].doc.Paths.Set("/providers/Microsoft.ContainerService/managedClusters/{resourceName}", pathItem)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindResource(tt.doc, tt.resourceType)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestFindResourceNameSchema(t *testing.T) {
	t.Parallel()

	maxLen := uint64(63)
	doc := &openapi3.T{Paths: &openapi3.Paths{}}

	pathItem := &openapi3.PathItem{
		Put: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name: "resourceName",
					In:   "path",
					Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
						Type:      &openapi3.Types{"string"},
						Pattern:   "^[a-z0-9-]+$",
						MinLength: 1,
						MaxLength: &maxLen,
					}},
				}},
			},
		},
	}

	doc.Paths.Set("/providers/Microsoft.ContainerService/managedClusters/{resourceName}", pathItem)

	schema, err := FindResourceNameSchema(doc, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.NotNil(t, schema.Type)
	assert.Contains(t, *schema.Type, "string")
	assert.Equal(t, "^[a-z0-9-]+$", schema.Pattern)
	assert.Equal(t, uint64(1), schema.MinLength)
	require.NotNil(t, schema.MaxLength)
	assert.Equal(t, maxLen, *schema.MaxLength)
}

func TestFindResourceNameSchema_SwaggerV2ParameterExtensions(t *testing.T) {
	t.Parallel()

	doc := &openapi3.T{Paths: &openapi3.Paths{}}

	pathItem := &openapi3.PathItem{
		Put: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name: "resourceName",
					In:   "path",
					// Swagger/OpenAPI v2-style parameter constraints are preserved by kin-openapi
					// under Extensions when no parameter schema exists.
					Extensions: map[string]interface{}{
						"type":      "string",
						"pattern":   "^[a-z0-9-]+$",
						"minLength": float64(1),
						"maxLength": float64(63),
					},
				}},
			},
		},
	}

	doc.Paths.Set("/providers/Microsoft.ContainerService/managedClusters/{resourceName}", pathItem)

	schema, err := FindResourceNameSchema(doc, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)
	require.NotNil(t, schema)

	require.NotNil(t, schema.Type)
	assert.Contains(t, *schema.Type, "string")
	assert.Equal(t, "^[a-z0-9-]+$", schema.Pattern)
	assert.Equal(t, uint64(1), schema.MinLength)
	require.NotNil(t, schema.MaxLength)
	assert.Equal(t, uint64(63), *schema.MaxLength)
}

func TestAzureARMInstancePathInfo(t *testing.T) {
	t.Parallel()

	t.Run("top-level instance path", func(t *testing.T) {
		t.Parallel()

		resourceType, nameParam, ok := azureARMInstancePathInfo("/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets/{widgetName}")
		require.True(t, ok)
		assert.Equal(t, "Microsoft.Test/widgets", resourceType)
		assert.Equal(t, "widgetName", nameParam)
	})

	t.Run("nested instance path", func(t *testing.T) {
		t.Parallel()

		resourceType, nameParam, ok := azureARMInstancePathInfo("/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets/{widgetName}/child/{childName}")
		require.True(t, ok)
		assert.Equal(t, "Microsoft.Test/widgets/child", resourceType)
		assert.Equal(t, "childName", nameParam)
	})

	t.Run("singleton constant instance path", func(t *testing.T) {
		t.Parallel()

		resourceType, nameParam, ok := azureARMInstancePathInfo("/providers/Microsoft.Storage/storageAccounts/{accountName}/blobServices/default")
		require.True(t, ok)
		assert.Equal(t, "Microsoft.Storage/storageAccounts/blobServices", resourceType)
		assert.Equal(t, "default", nameParam)
	})

	t.Run("nested path with singleton segment", func(t *testing.T) {
		t.Parallel()

		resourceType, nameParam, ok := azureARMInstancePathInfo("/providers/Microsoft.Storage/storageAccounts/{accountName}/blobServices/default/containers/{containerName}")
		require.True(t, ok)
		assert.Equal(t, "Microsoft.Storage/storageAccounts/blobServices/containers", resourceType)
		assert.Equal(t, "containerName", nameParam)
	})

	t.Run("collection path rejected", func(t *testing.T) {
		t.Parallel()

		_, _, ok := azureARMInstancePathInfo("/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets")
		assert.False(t, ok)
	})

	t.Run("parameterized provider rejected", func(t *testing.T) {
		t.Parallel()

		_, _, ok := azureARMInstancePathInfo("/providers/{providerNamespace}/widgets/{widgetName}")
		assert.False(t, ok)
	})

	t.Run("parameterized type rejected", func(t *testing.T) {
		t.Parallel()

		_, _, ok := azureARMInstancePathInfo("/providers/Microsoft.Test/{typeName}/{widgetName}")
		assert.False(t, ok)
	})
}

func TestNavigateSchema(t *testing.T) {
	rootSchema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"prop1": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
			"readOnlyRoot": {
				Value: &openapi3.Schema{
					Type:     &openapi3.Types{"string"},
					ReadOnly: true,
				},
			},
			"nested": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"child": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name    string
		schema  *openapi3.Schema
		path    string
		want    *openapi3.Schema
		wantErr bool
	}{
		{
			name:    "Root path",
			schema:  rootSchema,
			path:    "",
			want:    rootSchema,
			wantErr: false,
		},
		{
			name:    "Direct property",
			schema:  rootSchema,
			path:    "prop1",
			want:    rootSchema.Properties["prop1"].Value,
			wantErr: false,
		},
		{
			name:    "Nested property",
			schema:  rootSchema,
			path:    "nested.child",
			want:    rootSchema.Properties["nested"].Value.Properties["child"].Value,
			wantErr: false,
		},
		{
			name:    "Read-only root property",
			schema:  rootSchema,
			path:    "readOnlyRoot",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "Invalid path",
			schema:  rootSchema,
			path:    "nonexistent",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Invalid nested path",
			schema:  rootSchema,
			path:    "nested.nonexistent",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NavigateSchema(tt.schema, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLoadSpec_InvalidPath(t *testing.T) {
	_, err := LoadSpec("nonexistent_file.json")
	require.Error(t, err)
}
