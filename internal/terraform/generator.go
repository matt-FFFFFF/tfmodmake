// Package terraform provides functions to generate Terraform variable and local definitions from OpenAPI schemas.
package terraform

import "github.com/getkin/kin-openapi/openapi3"

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
//
// The optional nameSchema parameter is used to attach validations to the top-level "name" variable.
func Generate(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool, supportsLocation bool, nameSchema *openapi3.Schema) error {
	hasSchema := schema != nil

	// Collect secret fields from schema
	var secrets []secretField
	if hasSchema {
		secrets = collectSecretFields(schema, "")
	}

	if err := generateTerraform(); err != nil {
		return err
	}
	if err := generateVariables(schema, supportsTags, supportsLocation, secrets, nameSchema); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(schema, localName, secrets); err != nil {
			return err
		}
	}
	if err := generateMain(schema, resourceType, apiVersion, localName, supportsTags, supportsLocation, hasSchema, secrets); err != nil {
		return err
	}
	if err := generateOutputs(); err != nil {
		return err
	}
	return nil
}

// SupportsTags reports whether the schema includes a writable "tags" property, following allOf inheritance.
func SupportsTags(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "tags")
}

// SupportsLocation reports whether the schema includes a writable "location" property, following allOf inheritance.
func SupportsLocation(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "location")
}
