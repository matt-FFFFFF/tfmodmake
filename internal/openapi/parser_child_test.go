package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func TestFindResource_ExcludesChildResources(t *testing.T) {
	doc := &openapi3.T{
		Paths: &openapi3.Paths{},
	}

	// Parent resource schema
	parentSchema := &openapi3.Schema{
		Type:        &openapi3.Types{"object"},
		Description: "Parent Resource",
	}

	// Child resource schema
	childSchema := &openapi3.Schema{
		Type:        &openapi3.Types{"object"},
		Description: "Child Resource",
	}

	// Add parent path
	parentPathItem := &openapi3.PathItem{
		Put: &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: parentSchema,
							},
						},
					},
				},
			},
		},
	}
	doc.Paths.Set("/providers/Microsoft.ContainerService/managedClusters/{resourceName}", parentPathItem)

	// Add child path
	childPathItem := &openapi3.PathItem{
		Put: &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: childSchema,
							},
						},
					},
				},
			},
		},
	}
	doc.Paths.Set("/providers/Microsoft.ContainerService/managedClusters/{resourceName}/agentPools/{agentPoolName}", childPathItem)

	resourceType := "Microsoft.ContainerService/managedClusters"

	// We want to find the parent schema, not the child schema
	// Since map iteration order is random, we can't rely on order.
	// But the current implementation might pick the child if it encounters it.
	// We want to ensure it ALWAYS picks the parent.

	// To verify the bug, we might need to run this multiple times or force the order if possible.
	// But better, let's just implement the fix and verify it works.

	schema, err := FindResource(doc, resourceType)
	assert.NoError(t, err)
	assert.Equal(t, parentSchema, schema, "Should return parent schema, not child")
}
