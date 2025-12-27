package terraform

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

func generateTerraform() error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	tfBlock := body.AppendNewBlock("terraform", nil)
	tfBody := tfBlock.Body()
	tfBody.SetAttributeValue("required_version", cty.StringVal("~> 1.12"))

	providers := tfBody.AppendNewBlock("required_providers", nil)
	providers.Body().SetAttributeValue("azapi", cty.ObjectVal(map[string]cty.Value{
		"source":  cty.StringVal("azure/azapi"),
		"version": cty.StringVal("~> 2.7"),
	}))

	return hclgen.WriteFile("terraform.tf", file)
}
