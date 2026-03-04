// Package terraform provides functions to generate Terraform variable and local definitions from resource schemas.
package terraform

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/schema"
)

// GeneratorOption is a function that configures the generator.
type GeneratorOption func(*generatorOptions)

type generatorOptions struct {
	schema           *schema.ResourceSchema
	resourceType     string
	localName        string
	apiVersion       string
	moduleNamePrefix string
	outputDir        string
}

// WithResourceSchema sets the resource schema for generation.
func WithResourceSchema(rs *schema.ResourceSchema) GeneratorOption {
	return func(o *generatorOptions) {
		o.schema = rs
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
		if result.Schema != nil {
			o.apiVersion = result.Schema.APIVersion
		}
	}
}

// InterfaceCapabilities represents which AVM interface scaffolding should be generated.
type InterfaceCapabilities struct {
	SupportsPrivateEndpoints   bool
	SupportsDiagnostics        bool
	SupportsCustomerManagedKey bool
	SupportsManagedIdentity    bool
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
	supportsTags := SupportsTags(o.schema)
	supportsLocation := SupportsLocation(o.schema)
	hasDiscriminator := schema.HasDiscriminator(o.schema)

	// Build interface capabilities from schema
	caps := InterfaceCapabilities{
		SupportsManagedIdentity: supportsIdentity,
	}

	// Collect secret fields from schema
	var secrets []secretField
	if hasSchema {
		secrets = collectSecretFields(o.schema)
	}

	if err := generateTerraform(o.outputDir); err != nil {
		return err
	}
	if err := generateVariables(o.schema, supportsTags, supportsLocation, supportsIdentity, secrets, caps, o.moduleNamePrefix, o.outputDir); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(o.schema, o.localName, supportsIdentity, secrets, o.resourceType, caps, o.moduleNamePrefix, o.outputDir); err != nil {
			return err
		}
	}
	if err := generateMain(o.schema, o.resourceType, o.apiVersion, o.localName, supportsTags, supportsLocation, supportsIdentity, hasSchema, hasDiscriminator, secrets, o.outputDir); err != nil {
		return err
	}
	if err := generateOutputs(o.schema, o.outputDir); err != nil {
		return err
	}
	return nil
}

// GenerateInterfacesFile generates main.interfaces.tf with AVM interfaces module wiring.
// This function can be called separately to opt-in to AVM interfaces scaffolding.
func GenerateInterfacesFile(resourceType string, rs *schema.ResourceSchema, outputDir string) error {
	caps := InterfaceCapabilities{
		SupportsManagedIdentity: rs != nil && rs.SupportsIdentity,
	}
	return generateInterfaces(caps, outputDir)
}

// SupportsIdentity reports whether the schema supports configuring managed identity.
func SupportsIdentity(rs *schema.ResourceSchema) bool {
	return rs != nil && rs.SupportsIdentity
}

// SupportsTags reports whether the schema includes a writable "tags" property.
func SupportsTags(rs *schema.ResourceSchema) bool {
	return rs != nil && rs.SupportsTags
}

// SupportsLocation reports whether the schema includes a writable "location" property.
func SupportsLocation(rs *schema.ResourceSchema) bool {
	return rs != nil && rs.SupportsLocation
}

// GeneratedModule holds all generated HCL files in memory, enabling comparison
// without writing to disk.
type GeneratedModule struct {
	Terraform *hclwrite.File
	Variables *hclwrite.File
	Locals    *hclwrite.File
	Main      *hclwrite.File
	Outputs   *hclwrite.File
}

// GenerateInMemory runs the generation pipeline and returns all files in memory
// without writing to disk. This is used by the update command to produce baseline
// and new-version outputs for comparison.
func GenerateInMemory(resourceType string, opts ...GeneratorOption) (*GeneratedModule, error) {
	o := &generatorOptions{
		resourceType: resourceType,
		outputDir:    ".",
		localName:    "resource_body",
	}
	for _, opt := range opts {
		opt(o)
	}

	hasSchema := o.schema != nil
	supportsIdentity := SupportsIdentity(o.schema)
	supportsTags := SupportsTags(o.schema)
	supportsLocation := SupportsLocation(o.schema)
	hasDiscriminator := schema.HasDiscriminator(o.schema)

	caps := InterfaceCapabilities{
		SupportsManagedIdentity: supportsIdentity,
	}

	var secrets []secretField
	if hasSchema {
		secrets = collectSecretFields(o.schema)
	}

	mod := &GeneratedModule{
		Terraform: buildTerraform(),
		Outputs:   buildOutputs(o.schema),
	}

	var err error
	mod.Variables, err = buildVariables(o.schema, supportsTags, supportsLocation, supportsIdentity, secrets, caps, o.moduleNamePrefix)
	if err != nil {
		return nil, fmt.Errorf("building variables: %w", err)
	}

	if hasSchema {
		mod.Locals, err = buildLocals(o.schema, o.localName, supportsIdentity, secrets, o.resourceType, caps, o.moduleNamePrefix)
		if err != nil {
			return nil, fmt.Errorf("building locals: %w", err)
		}
	}

	mod.Main = buildMain(o.schema, o.resourceType, o.apiVersion, o.localName, supportsTags, supportsLocation, supportsIdentity, hasSchema, hasDiscriminator, secrets)

	return mod, nil
}
