// Package terraform provides functions to generate Terraform variable and local definitions from OpenAPI schemas.
package terraform

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
)

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
//
// The optional nameSchema parameter is used to attach validations to the top-level "name" variable.
// The spec parameter is used to detect which AVM interface capabilities are supported.
// The optional moduleNamePrefix is used to rename variables that conflict with Terraform module meta-arguments (e.g., "version" -> "dapr_component_version").
// Files are written to the current directory.
func Generate(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool, supportsLocation bool, nameSchema *openapi3.Schema, spec *openapi3.T) error {
	return GenerateWithContext(schema, resourceType, localName, apiVersion, supportsTags, supportsLocation, nameSchema, spec, "", ".")
}

// GenerateWithContext is like Generate but accepts a moduleNamePrefix for renaming reserved variable names and an outputDir for where to write files.
func GenerateWithContext(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool, supportsLocation bool, nameSchema *openapi3.Schema, spec *openapi3.T, moduleNamePrefix string, outputDir string) error {
	hasSchema := schema != nil
	supportsIdentity := SupportsIdentity(schema)

	// Detect interface capabilities from spec
	var caps openapi.InterfaceCapabilities
	if spec != nil {
		caps = openapi.DetectInterfaceCapabilities(spec, resourceType)
	}

	// Collect secret fields from schema
	var secrets []secretField
	if hasSchema {
		var err error
		secrets, err = collectSecretFields(schema, "")
		if err != nil {
			return fmt.Errorf("collecting secret fields: %w", err)
		}
	}

	if err := generateTerraform(outputDir); err != nil {
		return err
	}
	if err := generateVariables(schema, supportsTags, supportsLocation, supportsIdentity, secrets, nameSchema, caps, moduleNamePrefix, outputDir); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(schema, localName, supportsIdentity, secrets, resourceType, caps, moduleNamePrefix, outputDir); err != nil {
			return err
		}
	}
	if err := generateMain(schema, resourceType, apiVersion, localName, supportsTags, supportsLocation, supportsIdentity, hasSchema, secrets, outputDir); err != nil {
		return err
	}
	if err := generateOutputs(schema, outputDir); err != nil {
		return err
	}
	return nil
}

// GenerateInterfacesFile generates main.interfaces.tf with AVM interfaces module wiring.
// This function can be called separately to opt-in to AVM interfaces scaffolding.
func GenerateInterfacesFile(resourceType string, spec *openapi3.T, outputDir string) error {
	// Detect interface capabilities from spec
	var caps openapi.InterfaceCapabilities
	if spec != nil {
		caps = openapi.DetectInterfaceCapabilities(spec, resourceType)
	}
	return generateInterfaces(caps, outputDir)
}

// SupportsIdentity reports whether the schema supports configuring managed identity in a standard ARM pattern.
//
// We gate identity generation on the presence of the typical writable fields used by ARM identity:
//   - identity.type
//   - identity.userAssignedIdentities
//
// This avoids generating identity scaffolding for schemas that only expose read-only identity metadata.
func SupportsIdentity(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}
	return hasWritableProperty(schema, "identity.type") || hasWritableProperty(schema, "identity.userAssignedIdentities")
}

// SupportsTags reports whether the schema includes a writable "tags" property, following allOf inheritance.
func SupportsTags(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "tags")
}

// SupportsLocation reports whether the schema includes a writable "location" property, following allOf inheritance.
func SupportsLocation(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "location")
}
