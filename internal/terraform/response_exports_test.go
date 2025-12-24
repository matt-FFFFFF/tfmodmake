package terraform

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func TestExtractReadOnlyPaths(t *testing.T) {
	tests := []struct {
		name     string
		schema   *openapi3.Schema
		expected []string
	}{
		{
			name: "simple readOnly string properties",
			schema: &openapi3.Schema{
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
			},
			expected: []string{"id"},
		},
		{
			name: "nested readOnly properties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"properties": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: map[string]*openapi3.SchemaRef{
								"defaultDomain": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{"string"},
										ReadOnly: true,
									},
								},
								"staticIp": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{"string"},
										ReadOnly: true,
									},
								},
								"provisioningState": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{"string"},
										ReadOnly: true,
									},
								},
								"writableField": {
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"string"},
									},
								},
							},
						},
					},
					"identity": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: map[string]*openapi3.SchemaRef{
								"principalId": {
									Value: &openapi3.Schema{
										Type:     &openapi3.Types{"string"},
										ReadOnly: true,
									},
								},
							},
						},
					},
				},
			},
			expected: []string{
				"identity.principalId",
				"properties.defaultDomain",
				"properties.provisioningState",
				"properties.staticIp",
			},
		},
		{
			name: "readOnly number and boolean types",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"count": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"integer"},
							ReadOnly: true,
						},
					},
					"percentage": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"number"},
							ReadOnly: true,
						},
					},
					"enabled": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"boolean"},
							ReadOnly: true,
						},
					},
				},
			},
			expected: []string{"count", "enabled", "percentage"},
		},
		{
			name: "excludes readOnly objects and arrays",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"readOnlyObject": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"object"},
							ReadOnly: true,
							Properties: map[string]*openapi3.SchemaRef{
								"nested": {
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"string"},
									},
								},
							},
						},
					},
					"readOnlyArray": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"array"},
							ReadOnly: true,
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
								},
							},
						},
					},
					"readOnlyScalar": {
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"string"},
							ReadOnly: true,
						},
					},
				},
			},
			expected: []string{"readOnlyScalar"},
		},
		{
			name:     "nil schema returns empty list",
			schema:   nil,
			expected: nil,
		},
		{
			name: "empty schema returns empty list",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReadOnlyPaths(tt.schema)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFilterBlocklistedPaths(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected []string
	}{
		{
			name: "filters array-indexed paths",
			paths: []string{
				"properties.defaultDomain",
				"properties.agentPoolProfiles[0].provisioningState",
				"properties.agentPoolProfiles[0].status.provisioningError.message",
				"identity.principalId",
			},
			expected: []string{
				"properties.defaultDomain",
				"identity.principalId",
			},
		},
		{
			name: "filters status paths",
			paths: []string{
				"properties.defaultDomain",
				"properties.status.phase",
				"properties.networkProfile.status.ready",
				"identity.principalId",
			},
			expected: []string{
				"properties.defaultDomain",
				"identity.principalId",
			},
		},
		{
			name: "filters provisioningError paths",
			paths: []string{
				"properties.defaultDomain",
				"properties.provisioningError.code",
				"properties.provisioningError.message",
				"identity.principalId",
			},
			expected: []string{
				"properties.defaultDomain",
				"identity.principalId",
			},
		},
		{
			name: "filters eTag fields",
			paths: []string{
				"properties.defaultDomain",
				"eTag",
				"properties.eTag",
				"properties.etag",
				"identity.principalId",
			},
			expected: []string{
				"properties.defaultDomain",
				"identity.principalId",
			},
		},
		{
			name: "filters timestamp fields",
			paths: []string{
				"properties.defaultDomain",
				"properties.createdAt",
				"properties.lastModified",
				"properties.timestamp",
				"identity.principalId",
			},
			expected: []string{
				"properties.defaultDomain",
				"identity.principalId",
			},
		},
		{
			name: "allows provisioningState (not provisioningError)",
			paths: []string{
				"properties.provisioningState",
				"properties.provisioningError.code",
			},
			expected: []string{
				"properties.provisioningState",
			},
		},
		{
			name: "empty input returns empty output",
			paths: []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterBlocklistedPaths(tt.paths)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestIsLeafScalar(t *testing.T) {
	tests := []struct {
		name     string
		schema   *openapi3.Schema
		expected bool
	}{
		{
			name:     "string is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"string"}},
			expected: true,
		},
		{
			name:     "number is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"number"}},
			expected: true,
		},
		{
			name:     "integer is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"integer"}},
			expected: true,
		},
		{
			name:     "boolean is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"boolean"}},
			expected: true,
		},
		{
			name:     "object is not scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"object"}},
			expected: false,
		},
		{
			name:     "array is not scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"array"}},
			expected: false,
		},
		{
			name:     "nil schema is not scalar",
			schema:   nil,
			expected: false,
		},
		{
			name:     "schema with nil type is not scalar",
			schema:   &openapi3.Schema{},
			expected: false,
		},
		{
			name:     "nullable string is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"null", "string"}},
			expected: true,
		},
		{
			name:     "nullable integer is scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"null", "integer"}},
			expected: true,
		},
		{
			name:     "nullable object is not scalar",
			schema:   &openapi3.Schema{Type: &openapi3.Types{"null", "object"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLeafScalar(tt.schema)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestShouldBlockPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "allows normal path",
			path:     "properties.defaultDomain",
			expected: false,
		},
		{
			name:     "blocks array index",
			path:     "properties.agentPoolProfiles[0].name",
			expected: true,
		},
		{
			name:     "blocks status path",
			path:     "properties.status.ready",
			expected: true,
		},
		{
			name:     "blocks provisioningError path",
			path:     "properties.provisioningError.code",
			expected: true,
		},
		{
			name:     "blocks eTag",
			path:     "eTag",
			expected: true,
		},
		{
			name:     "blocks etag (lowercase)",
			path:     "properties.etag",
			expected: true,
		},
		{
			name:     "blocks timestamp",
			path:     "properties.timestamp",
			expected: true,
		},
		{
			name:     "blocks createdAt",
			path:     "properties.createdAt",
			expected: true,
		},
		{
			name:     "blocks lastModified",
			path:     "properties.lastModified",
			expected: true,
		},
		{
			name:     "allows provisioningState",
			path:     "properties.provisioningState",
			expected: false,
		},
		{
			name:     "blocks root-level status",
			path:     "status",
			expected: true,
		},
		{
			name:     "blocks root-level status with property",
			path:     "status.phase",
			expected: true,
		},
		{
			name:     "blocks root-level provisioningError",
			path:     "provisioningError",
			expected: true,
		},
		{
			name:     "blocks root-level provisioningError with property",
			path:     "provisioningError.code",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldBlockPath(tt.path)
			assert.Equal(t, tt.expected, got)
		})
	}
}
