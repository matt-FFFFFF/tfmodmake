package terraform

import (
	"testing"

	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/stretchr/testify/assert"
)

func TestExtractComputedPaths(t *testing.T) {
	tests := []struct {
		name     string
		rs       *schema.ResourceSchema
		expected []string
	}{
		{
			name: "simple readOnly string properties",
			rs: &schema.ResourceSchema{
				Properties: map[string]*schema.Property{
					"id": {
						Name:     "id",
						Type:     schema.TypeString,
						ReadOnly: true,
					},
					"name": {
						Name: "name",
						Type: schema.TypeString,
					},
				},
			},
			expected: []string{},
		},
		{
			name: "nested readOnly properties",
			rs: &schema.ResourceSchema{
				Properties: map[string]*schema.Property{
					"properties": {
						Name: "properties",
						Type: schema.TypeObject,
						Children: map[string]*schema.Property{
							"defaultDomain": {
								Name:     "defaultDomain",
								Type:     schema.TypeString,
								ReadOnly: true,
							},
							"staticIp": {
								Name:     "staticIp",
								Type:     schema.TypeString,
								ReadOnly: true,
							},
							"provisioningState": {
								Name:     "provisioningState",
								Type:     schema.TypeString,
								ReadOnly: true,
							},
							"writableField": {
								Name: "writableField",
								Type: schema.TypeString,
							},
						},
					},
					"identity": {
						Name: "identity",
						Type: schema.TypeObject,
						Children: map[string]*schema.Property{
							"principalId": {
								Name:     "principalId",
								Type:     schema.TypeString,
								ReadOnly: true,
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
			rs: &schema.ResourceSchema{
				Properties: map[string]*schema.Property{
					"count": {
						Name:     "count",
						Type:     schema.TypeInteger,
						ReadOnly: true,
					},
					"percentage": {
						Name:     "percentage",
						Type:     schema.TypeInteger,
						ReadOnly: true,
					},
					"enabled": {
						Name:     "enabled",
						Type:     schema.TypeBoolean,
						ReadOnly: true,
					},
				},
			},
			expected: []string{"count", "enabled", "percentage"},
		},
		{
			name: "includes computed objects and arrays",
			rs: &schema.ResourceSchema{
				Properties: map[string]*schema.Property{
					"readOnlyObject": {
						Name:     "readOnlyObject",
						Type:     schema.TypeObject,
						ReadOnly: true,
						Children: map[string]*schema.Property{
							"nested": {
								Name: "nested",
								Type: schema.TypeString,
							},
						},
					},
					"readOnlyArray": {
						Name:     "readOnlyArray",
						Type:     schema.TypeArray,
						ReadOnly: true,
						ItemType: &schema.Property{Type: schema.TypeString},
					},
					"readOnlyScalar": {
						Name:     "readOnlyScalar",
						Type:     schema.TypeString,
						ReadOnly: true,
					},
				},
			},
			expected: []string{"readOnlyArray", "readOnlyObject", "readOnlyScalar"},
		},
		{
			name:     "nil schema returns empty list",
			rs:       nil,
			expected: nil,
		},
		{
			name: "empty schema returns empty list",
			rs: &schema.ResourceSchema{
				Properties: map[string]*schema.Property{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractComputedPaths(tt.rs)
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
			name:     "empty input returns empty output",
			paths:    []string{},
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
