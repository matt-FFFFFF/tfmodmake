package terraform

import (
	"context"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
)

// ResourceLoadResult contains all information needed to generate a Terraform module
// for a resource loaded from OpenAPI specs.
type ResourceLoadResult struct {
	Schema           *openapi3.Schema
	NameSchema       *openapi3.Schema
	Doc              *openapi3.T
	APIVersion       string
	SupportsTags     bool
	SupportsLocation bool
}

// LoadResource attempts to find and load a resource type from a list of specs.
// It returns the first successful match or an error with details about failures.
func LoadResource(ctx context.Context, specs []string, resourceType string) (*ResourceLoadResult, error) {
	var loadErrors []string
	var searchErrors []string

	for _, specPath := range specs {
		// Check context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		loadedDoc, err := openapi.LoadSpec(specPath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue // Try next spec
		}

		// Try to find the resource in this spec
		foundSchema, err := openapi.FindResource(loadedDoc, resourceType)
		if err != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue // Try next spec
		}

		// Found the resource! Build result
		result := &ResourceLoadResult{
			Schema: foundSchema,
			Doc:    loadedDoc,
		}

		// Get API version
		if loadedDoc.Info != nil {
			result.APIVersion = loadedDoc.Info.Version
		}

		// Get name schema
		result.NameSchema, _ = openapi.FindResourceNameSchema(loadedDoc, resourceType)

		// Apply writability overrides
		openapi.AnnotateSchemaRefOrigins(result.Schema)
		if resolver, err := openapi.NewPropertyWritabilityResolver(specPath); err == nil && resolver != nil {
			openapi.ApplyPropertyWritabilityOverrides(result.Schema, resolver)
		}

		// Check for tags and location support
		result.SupportsTags = SupportsTags(result.Schema)
		result.SupportsLocation = SupportsLocation(result.Schema)

		return result, nil
	}

	// Resource not found in any spec
	return nil, buildResourceNotFoundError(resourceType, loadErrors, searchErrors)
}

func buildResourceNotFoundError(resourceType string, loadErrors, searchErrors []string) error {
	errMsg := fmt.Sprintf("resource type %s not found in any of the provided specs", resourceType)
	if len(loadErrors) > 0 {
		errMsg += fmt.Sprintf("\n\nSpec load errors:\n%s", strings.Join(loadErrors, "\n"))
	}
	if len(searchErrors) > 0 {
		errMsg += fmt.Sprintf("\n\nSpecs checked:\n%s", strings.Join(searchErrors, "\n"))
	}
	return fmt.Errorf("%s", errMsg)
}
