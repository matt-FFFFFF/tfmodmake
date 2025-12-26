package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsChildOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		childType string
		parent    string
		maxDepth  int
		want      bool
	}{
		{
			name:      "direct child",
			childType: "Microsoft.App/managedEnvironments/certificates",
			parent:    "Microsoft.App/managedEnvironments",
			maxDepth:  1,
			want:      true,
		},
		{
			name:      "grandchild excluded when depth=1",
			childType: "Microsoft.App/managedEnvironments/certificates/bindings",
			parent:    "Microsoft.App/managedEnvironments",
			maxDepth:  1,
			want:      false,
		},
		{
			name:      "grandchild included when depth=2",
			childType: "Microsoft.App/managedEnvironments/certificates/bindings",
			parent:    "Microsoft.App/managedEnvironments",
			maxDepth:  2,
			want:      true,
		},
		{
			name:      "not a child",
			childType: "Microsoft.App/containerApps",
			parent:    "Microsoft.App/managedEnvironments",
			maxDepth:  1,
			want:      false,
		},
		{
			name:      "parent itself excluded",
			childType: "Microsoft.App/managedEnvironments",
			parent:    "Microsoft.App/managedEnvironments",
			maxDepth:  1,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isChildOf(tt.childType, tt.parent, tt.maxDepth)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasRequestBodySchema(t *testing.T) {
	t.Parallel()

	t.Run("OpenAPI 3 RequestBody", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{
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
		}

		assert.True(t, hasRequestBodySchema(op))
	})

	t.Run("Swagger v2 body parameter", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{
					Value: &openapi3.Parameter{
						In: "body",
						Schema: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
							},
						},
					},
				},
			},
		}

		assert.True(t, hasRequestBodySchema(op))
	})

	t.Run("no request body", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{}
		assert.False(t, hasRequestBodySchema(op))
	})

	t.Run("nil operation", func(t *testing.T) {
		t.Parallel()

		assert.False(t, hasRequestBodySchema(nil))
	})

	t.Run("OpenAPI 3 RequestBody with $ref schema", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Ref: "#/components/schemas/SomeSchema",
							},
						},
					},
				},
			},
		}

		assert.True(t, hasRequestBodySchema(op), "should detect $ref schema")
	})

	t.Run("OpenAPI 3 with merge-patch+json content type", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/merge-patch+json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
								},
							},
						},
					},
				},
			},
		}

		assert.True(t, hasRequestBodySchema(op), "should detect merge-patch+json content type")
	})

	t.Run("Swagger v2 body parameter with $ref", func(t *testing.T) {
		t.Parallel()

		op := &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{
					Value: &openapi3.Parameter{
						In: "body",
						Schema: &openapi3.SchemaRef{
							Ref: "#/definitions/SomeDefinition",
						},
					},
				},
			},
		}

		assert.True(t, hasRequestBodySchema(op), "should detect $ref in Swagger v2 body parameter")
	})
}

func TestExtractAPIVersion(t *testing.T) {
	t.Parallel()

	t.Run("from doc.Info.Version", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info: &openapi3.Info{
				Version: "2024-01-01",
			},
		}

		version := extractAPIVersion(doc, "some/path/spec.json")
		assert.Equal(t, "2024-01-01", version)
	})

	t.Run("from spec path - stable version", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{}
		path := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/stable/2024-03-01/ManagedEnvironments.json"

		version := extractAPIVersion(doc, path)
		assert.Equal(t, "2024-03-01", version)
	})

	t.Run("from spec path - preview version", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{}
		path := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/preview/2025-10-02-preview/ManagedEnvironments.json"

		version := extractAPIVersion(doc, path)
		assert.Equal(t, "2025-10-02-preview", version)
	})

	t.Run("fallback to empty string", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{}
		version := extractAPIVersion(doc, "some/unknown/path.json")
		assert.Equal(t, "", version)
	})

	t.Run("prefers doc.Info.Version over path", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info: &openapi3.Info{
				Version: "2024-05-01",
			},
		}
		path := "https://example.com/2024-01-01/spec.json"

		version := extractAPIVersion(doc, path)
		assert.Equal(t, "2024-05-01", version, "should prefer doc.Info.Version")
	})
}

func TestDiscoverChildrenInSpec(t *testing.T) {
	t.Parallel()

	t.Run("discovers direct children", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info:  &openapi3.Info{Version: "2024-01-01"},
			Paths: &openapi3.Paths{},
		}

		// Parent resource
		parentPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}"
		doc.Paths.Set(parentPath, &openapi3.PathItem{
			Put: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
								},
							},
						},
					},
				},
			},
		})

		// Child resource with PUT and body
		certPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/certificates/{certificateName}"
		doc.Paths.Set(certPath, &openapi3.PathItem{
			Put: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
								},
							},
						},
					},
				},
			},
			Get: &openapi3.Operation{},
		})

		childrenMap := make(map[string]*ChildResource)
		err := discoverChildrenInSpec(doc, "Microsoft.App/managedEnvironments", 1, "2024-01-01", childrenMap)
		require.NoError(t, err)

		require.Len(t, childrenMap, 1)
		child, exists := childrenMap["Microsoft.App/managedEnvironments/certificates"]
		require.True(t, exists)

		assert.Equal(t, "Microsoft.App/managedEnvironments/certificates", child.ResourceType)
		assert.True(t, child.IsDeployable)
		assert.Empty(t, child.DeployabilityReason)
		assert.Contains(t, child.Operations, "PUT")
		assert.Contains(t, child.Operations, "GET")
		assert.Equal(t, "2024-01-01", child.APIVersion)
		assert.Contains(t, child.ExamplePaths, certPath)
	})

	t.Run("filters GET-only resources", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info:  &openapi3.Info{Version: "2024-01-01"},
			Paths: &openapi3.Paths{},
		}

		// GET-only child resource (e.g., status endpoint)
		statusPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/status/{statusName}"
		doc.Paths.Set(statusPath, &openapi3.PathItem{
			Get: &openapi3.Operation{},
		})

		childrenMap := make(map[string]*ChildResource)
		err := discoverChildrenInSpec(doc, "Microsoft.App/managedEnvironments", 1, "2024-01-01", childrenMap)
		require.NoError(t, err)

		require.Len(t, childrenMap, 1)
		child := childrenMap["Microsoft.App/managedEnvironments/status"]

		assert.False(t, child.IsDeployable)
		assert.Equal(t, "GET-only resource", child.DeployabilityReason)
	})

	t.Run("filters PUT without body schema", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info:  &openapi3.Info{Version: "2024-01-01"},
			Paths: &openapi3.Paths{},
		}

		// PUT without body (action endpoint)
		actionPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/actions/{actionName}"
		doc.Paths.Set(actionPath, &openapi3.PathItem{
			Put: &openapi3.Operation{
				// No RequestBody
			},
		})

		childrenMap := make(map[string]*ChildResource)
		err := discoverChildrenInSpec(doc, "Microsoft.App/managedEnvironments", 1, "2024-01-01", childrenMap)
		require.NoError(t, err)

		require.Len(t, childrenMap, 1)
		child := childrenMap["Microsoft.App/managedEnvironments/actions"]

		assert.False(t, child.IsDeployable)
		assert.Equal(t, "No request body schema found", child.DeployabilityReason)
	})

	t.Run("excludes grandchildren when depth=1", func(t *testing.T) {
		t.Parallel()

		doc := &openapi3.T{
			Info:  &openapi3.Info{Version: "2024-01-01"},
			Paths: &openapi3.Paths{},
		}

		// Grandchild resource
		gcPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/certificates/{certificateName}/bindings/{bindingName}"
		doc.Paths.Set(gcPath, &openapi3.PathItem{
			Put: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
								},
							},
						},
					},
				},
			},
		})

		childrenMap := make(map[string]*ChildResource)
		err := discoverChildrenInSpec(doc, "Microsoft.App/managedEnvironments", 1, "2024-01-01", childrenMap)
		require.NoError(t, err)

		assert.Len(t, childrenMap, 0, "grandchildren should be excluded when depth=1")
	})

	t.Run("prefers later API version", func(t *testing.T) {
		t.Parallel()

		doc1 := &openapi3.T{
			Info:  &openapi3.Info{Version: "2023-01-01"},
			Paths: &openapi3.Paths{},
		}
		doc2 := &openapi3.T{
			Info:  &openapi3.Info{Version: "2024-01-01"},
			Paths: &openapi3.Paths{},
		}

		certPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/certificates/{certificateName}"
		pathItem := &openapi3.PathItem{
			Put: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
								},
							},
						},
					},
				},
			},
		}

		doc1.Paths.Set(certPath, pathItem)
		doc2.Paths.Set(certPath, pathItem)

		childrenMap := make(map[string]*ChildResource)

		// Process older version first
		err := discoverChildrenInSpec(doc1, "Microsoft.App/managedEnvironments", 1, "2023-01-01", childrenMap)
		require.NoError(t, err)

		child := childrenMap["Microsoft.App/managedEnvironments/certificates"]
		assert.Equal(t, "2023-01-01", child.APIVersion)

		// Process newer version
		err = discoverChildrenInSpec(doc2, "Microsoft.App/managedEnvironments", 1, "2024-01-01", childrenMap)
		require.NoError(t, err)

		child = childrenMap["Microsoft.App/managedEnvironments/certificates"]
		assert.Equal(t, "2024-01-01", child.APIVersion, "should prefer later API version")
	})
}

func TestDiscoverChildren(t *testing.T) {
	t.Run("requires at least one spec", func(t *testing.T) {
		opts := DiscoverChildrenOptions{
			Specs:  []string{},
			Parent: "Microsoft.App/managedEnvironments",
		}

		_, err := DiscoverChildren(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one spec")
	})

	t.Run("requires parent", func(t *testing.T) {
		opts := DiscoverChildrenOptions{
			Specs:  []string{"dummy.json"},
			Parent: "",
		}

		_, err := DiscoverChildren(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parent resource type must be provided")
	})
}
