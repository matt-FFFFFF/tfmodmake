package terraform

import (
	"context"
	"fmt"

	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
	"github.com/matt-FFFFFF/tfmodmake/schema"
)

// ResourceLoadResult contains all information needed to generate a Terraform module.
type ResourceLoadResult struct {
	Schema *schema.ResourceSchema
}

// LoadOption configures how a resource is loaded.
type LoadOption func(*loadOptions)

type loadOptions struct {
	apiVersion     string
	includePreview bool
}

// WithAPIVersionLoad sets a specific API version to load.
func WithAPIVersionLoad(version string) LoadOption {
	return func(o *loadOptions) {
		o.apiVersion = version
	}
}

// WithIncludePreview allows selecting preview API versions when no specific version is set.
func WithIncludePreview(include bool) LoadOption {
	return func(o *loadOptions) {
		o.includePreview = include
	}
}

// LoadResource loads a resource type using bicep-types-az data.
func LoadResource(ctx context.Context, resourceType string, opts ...LoadOption) (GeneratorOption, error) {
	lo := &loadOptions{}
	for _, opt := range opts {
		opt(lo)
	}

	loaded, err := bicepdata.LoadResource(ctx, resourceType, lo.apiVersion, lo.includePreview, nil)
	if err != nil {
		return nil, fmt.Errorf("loading resource %s: %w", resourceType, err)
	}

	rs, err := schema.ConvertResource(loaded)
	if err != nil {
		return nil, fmt.Errorf("converting resource %s: %w", resourceType, err)
	}

	return func(o *generatorOptions) {
		o.schema = rs
		o.apiVersion = rs.APIVersion
	}, nil
}
