package terraform

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

// generateOutputs creates the outputs.tf file with AVM-compliant outputs.
// Always includes the mandatory AVM outputs: resource_id and name.
// Additional outputs for computed/readOnly attributes should be added manually by module authors.
func generateOutputs() error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// AVM mandatory output: resource_id
	resourceID := body.AppendNewBlock("output", []string{"resource_id"})
	resourceIDBody := resourceID.Body()
	resourceIDBody.SetAttributeValue("description", cty.StringVal("The ID of the created resource."))
	resourceIDBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "id"))

	// AVM mandatory output: name
	name := body.AppendNewBlock("output", []string{"name"})
	nameBody := name.Body()
	nameBody.SetAttributeValue("description", cty.StringVal("The name of the created resource."))
	nameBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "name"))

	return hclgen.WriteFile("outputs.tf", file)
}
