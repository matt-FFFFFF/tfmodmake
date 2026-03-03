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

// LoadResource loads a resource type using bicep-types-az data.
// specs parameter is retained for backward compatibility but ignored.
func LoadResource(ctx context.Context, specs []string, resourceType string) (GeneratorOption, error) {
	loaded, err := bicepdata.LoadResource(ctx, resourceType, "", false, nil)
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
