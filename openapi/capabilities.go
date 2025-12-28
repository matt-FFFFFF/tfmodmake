package openapi

import (
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// InterfaceCapabilities represents which AVM interface scaffolding should be generated
// based on evidence from the REST API specification.
type InterfaceCapabilities struct {
	SupportsPrivateEndpoints  bool
	SupportsDiagnostics       bool
	SupportsCustomerManagedKey bool
	// Lock and RoleAssignments are ARM-level capabilities not detectable from individual resource specs
}

// DetectInterfaceCapabilities analyzes the OpenAPI spec to determine which AVM interfaces
// are supported by examining paths, schemas, and properties.
func DetectInterfaceCapabilities(spec *openapi3.T, resourceType string) InterfaceCapabilities {
	caps := InterfaceCapabilities{}

	if spec == nil {
		return caps
	}

	// Check for private endpoint support by looking for privateEndpointConnections or privateLinkResources paths
	caps.SupportsPrivateEndpoints = detectPrivateEndpointSupport(spec, resourceType)

	// Check for diagnostic settings support - Azure resources supporting diagnostics typically don't
	// declare it in their own spec; it's a generic Microsoft.Insights capability on most ARM resources
	// For now, we'll assume most resources support diagnostics unless we have specific evidence otherwise
	caps.SupportsDiagnostics = detectDiagnosticSupport(spec, resourceType)

	// Check for customer-managed key support by looking for encryption properties in the schema
	caps.SupportsCustomerManagedKey = detectCustomerManagedKeySupport(spec, resourceType)

	return caps
}

// detectPrivateEndpointSupport checks if the spec includes Private Link/Private Endpoint paths
func detectPrivateEndpointSupport(spec *openapi3.T, resourceType string) bool {
	if spec.Paths == nil {
		return false
	}

	// Look for paths containing privateEndpointConnections or privateLinkResources
	for path := range spec.Paths.Map() {
		pathLower := strings.ToLower(path)
		if strings.Contains(pathLower, "privateendpointconnections") ||
			strings.Contains(pathLower, "privatelinkresources") {
			return true
		}
	}

	// Also check if any schema definitions mention private endpoint properties
	if spec.Components != nil && spec.Components.Schemas != nil {
		for schemaName, schemaRef := range spec.Components.Schemas {
			if schemaRef == nil || schemaRef.Value == nil {
				continue
			}
			schemaNameLower := strings.ToLower(schemaName)
			if strings.Contains(schemaNameLower, "privateendpoint") ||
				strings.Contains(schemaNameLower, "privatelink") {
				return true
			}
		}
	}

	return false
}

// detectDiagnosticSupport checks if the resource supports diagnostic settings.
// Currently returns false by default since diagnostic settings are a generic ARM capability
// managed via Microsoft.Insights, not declared in individual resource provider specs.
func detectDiagnosticSupport(spec *openapi3.T, resourceType string) bool {
	// Diagnostic settings are managed via Microsoft.Insights provider and work on most ARM resources.
	// Individual resource provider specs don't typically declare diagnostic settings support.
	// We should NOT auto-generate this unless there's explicit evidence.
	return false
}

// detectCustomerManagedKeySupport checks if the schema includes encryption/customerManagedKey properties
func detectCustomerManagedKeySupport(spec *openapi3.T, resourceType string) bool {
	if spec.Components == nil || spec.Components.Schemas == nil {
		return false
	}

	// Check PUT operation request body schema for encryption properties
	for path, pathItem := range spec.Paths.Map() {
		if pathItem == nil {
			continue
		}

		// Only check PUT operations (create/update)
		if pathItem.Put == nil {
			continue
		}

		// Check if this path matches the resource type
		if !strings.Contains(strings.ToLower(path), strings.ToLower(resourceType)) {
			continue
		}

		// Check request body schema
		if pathItem.Put.RequestBody == nil || pathItem.Put.RequestBody.Value == nil {
			continue
		}

		for _, content := range pathItem.Put.RequestBody.Value.Content {
			if content.Schema == nil || content.Schema.Value == nil {
				continue
			}

			if hasEncryptionProperty(content.Schema.Value) {
				return true
			}
		}
	}

	return false
}

// hasEncryptionProperty recursively checks if a schema has encryption/customerManagedKey properties
func hasEncryptionProperty(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}

	// Get effective properties (handles allOf)
	props, err := GetEffectiveProperties(schema)
	if err != nil {
		return false
	}

	for propName, propRef := range props {
		if propRef == nil || propRef.Value == nil {
			continue
		}

		propNameLower := strings.ToLower(propName)
		
		// Check for encryption-related property names
		if propNameLower == "encryption" ||
			propNameLower == "customermanagedkey" ||
			strings.Contains(propNameLower, "encryptionkey") {
			return true
		}

		// Check nested properties object
		if propName == "properties" && propRef.Value.Type != nil && slices.Contains(*propRef.Value.Type, "object") {
			if hasEncryptionProperty(propRef.Value) {
				return true
			}
		}
	}

	return false
}
