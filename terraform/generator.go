// Package terraform provides functions to generate Terraform variable and local definitions from OpenAPI schemas.
package terraform

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
)

// GeneratorOption is a function that configures the generator.
type GeneratorOption func(*generatorOptions)

type generatorOptions struct {
	schema           *openapi3.Schema
	resourceType     string
	localName        string
	apiVersion       string
	supportsTags     bool
	supportsLocation bool
	spec             *openapi3.T
	moduleNamePrefix string
	outputDir        string
}

// WithSchema sets the OpenAPI schema for the resource.
func WithSchema(schema *openapi3.Schema) GeneratorOption {
	return func(o *generatorOptions) {
		o.schema = schema
	}
}

// WithLocalName sets the local variable name for the resource body.
func WithLocalName(name string) GeneratorOption {
	return func(o *generatorOptions) {
		o.localName = name
	}
}

// WithAPIVersion sets the API version for the resource.
func WithAPIVersion(version string) GeneratorOption {
	return func(o *generatorOptions) {
		o.apiVersion = version
	}
}

// WithSupportsTags sets whether the resource supports tags.
func WithSupportsTags(supports bool) GeneratorOption {
	return func(o *generatorOptions) {
		o.supportsTags = supports
	}
}

// WithSupportsLocation sets whether the resource supports location.
func WithSupportsLocation(supports bool) GeneratorOption {
	return func(o *generatorOptions) {
		o.supportsLocation = supports
	}
}

// WithSpec sets the full OpenAPI spec document.
func WithSpec(spec *openapi3.T) GeneratorOption {
	return func(o *generatorOptions) {
		o.spec = spec
	}
}

// WithModuleNamePrefix sets a prefix for module names to avoid conflicts.
func WithModuleNamePrefix(prefix string) GeneratorOption {
	return func(o *generatorOptions) {
		o.moduleNamePrefix = prefix
	}
}

// WithOutputDir sets the directory where files will be generated.
func WithOutputDir(dir string) GeneratorOption {
	return func(o *generatorOptions) {
		o.outputDir = dir
	}
}

// WithLoadResult sets multiple options from a ResourceLoadResult.
func WithLoadResult(result *ResourceLoadResult) GeneratorOption {
	return func(o *generatorOptions) {
		o.schema = result.Schema
		o.apiVersion = result.APIVersion
		o.supportsTags = result.SupportsTags
		o.supportsLocation = result.SupportsLocation
		o.spec = result.Doc
	}
}

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
func Generate(resourceType string, opts ...GeneratorOption) error {
	o := &generatorOptions{
		resourceType: resourceType,
		outputDir:    ".",
		localName:    "resource_body",
	}
	for _, opt := range opts {
		opt(o)
	}

	return generateWithOpts(o)
}

func generateWithOpts(o *generatorOptions) error {
	hasSchema := o.schema != nil
	supportsIdentity := SupportsIdentity(o.schema)

	// Detect interface capabilities from spec
	var caps openapi.InterfaceCapabilities
	var nameSchema *openapi3.Schema
	if o.spec != nil {
		caps = openapi.DetectInterfaceCapabilities(o.spec, o.resourceType)
		nameSchema, _ = openapi.FindResourceNameSchema(o.spec, o.resourceType)
	}

	// Collect secret fields from schema
	var secrets []secretField
	if hasSchema {
		var err error
		secrets, err = collectSecretFields(o.schema, "")
		if err != nil {
			return fmt.Errorf("collecting secret fields: %w", err)
		}
	}

	if err := generateTerraform(o.outputDir); err != nil {
		return err
	}
	if err := generateVariables(o.schema, o.supportsTags, o.supportsLocation, supportsIdentity, secrets, nameSchema, caps, o.moduleNamePrefix, o.outputDir); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(o.schema, o.localName, supportsIdentity, secrets, o.resourceType, caps, o.moduleNamePrefix, o.outputDir); err != nil {
			return err
		}
	}
	if err := generateMain(o.schema, o.resourceType, o.apiVersion, o.localName, o.supportsTags, o.supportsLocation, supportsIdentity, hasSchema, secrets, o.outputDir); err != nil {
		return err
	}
	if err := generateOutputs(o.schema, o.outputDir); err != nil {
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
