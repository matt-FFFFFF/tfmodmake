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
