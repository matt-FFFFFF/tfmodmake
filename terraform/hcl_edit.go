package terraform

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// UpdateVariableType replaces the type attribute of a variable block in a parsed HCL file.
func UpdateVariableType(file *hclwrite.File, varName string, newType hclwrite.Tokens) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "variable" {
			continue
		}
		labels := block.Labels()
		if len(labels) == 0 || labels[0] != varName {
			continue
		}
		block.Body().SetAttributeRaw("type", newType)
		return nil
	}
	return fmt.Errorf("variable %q not found", varName)
}

// AddVariableBlock appends a complete variable block (from a generated file) to the target file.
// It extracts the variable block for varName from sourceFile and appends it to targetFile.
func AddVariableBlock(targetFile, sourceFile *hclwrite.File, varName string) error {
	for _, block := range sourceFile.Body().Blocks() {
		if block.Type() != "variable" {
			continue
		}
		labels := block.Labels()
		if len(labels) == 0 || labels[0] != varName {
			continue
		}
		targetFile.Body().AppendNewline()
		targetFile.Body().AppendUnstructuredTokens(block.BuildTokens(nil))
		return nil
	}
	return fmt.Errorf("variable %q not found in source file", varName)
}

// RemoveVariableBlock removes a variable block by name from a parsed HCL file.
func RemoveVariableBlock(file *hclwrite.File, varName string) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "variable" {
			continue
		}
		labels := block.Labels()
		if len(labels) == 0 || labels[0] != varName {
			continue
		}
		file.Body().RemoveBlock(block)
		return nil
	}
	return fmt.Errorf("variable %q not found", varName)
}

// UpdateLocalAttribute replaces an attribute expression in the first locals block.
func UpdateLocalAttribute(file *hclwrite.File, attrName string, newExpr hclwrite.Tokens) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "locals" {
			continue
		}
		if block.Body().GetAttribute(attrName) == nil {
			continue
		}
		block.Body().SetAttributeRaw(attrName, newExpr)
		return nil
	}
	return fmt.Errorf("local attribute %q not found", attrName)
}

// AddLocalAttribute adds a new attribute to the first locals block.
func AddLocalAttribute(file *hclwrite.File, attrName string, expr hclwrite.Tokens) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "locals" {
			continue
		}
		block.Body().SetAttributeRaw(attrName, expr)
		return nil
	}
	return fmt.Errorf("no locals block found")
}

// RemoveLocalAttribute removes an attribute from the first locals block that contains it.
func RemoveLocalAttribute(file *hclwrite.File, attrName string) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "locals" {
			continue
		}
		if block.Body().GetAttribute(attrName) == nil {
			continue
		}
		block.Body().RemoveAttribute(attrName)
		return nil
	}
	return fmt.Errorf("local attribute %q not found", attrName)
}

// UpdateResourceTypeAttribute updates the type string in azapi_resource "this"
// from "ResourceType@OldVersion" to "ResourceType@NewVersion".
func UpdateResourceTypeAttribute(file *hclwrite.File, newTypeString string) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "azapi_resource" || labels[1] != "this" {
			continue
		}
		block.Body().SetAttributeValue("type", cty.StringVal(newTypeString))
		return nil
	}
	return fmt.Errorf("no azapi_resource \"this\" block found")
}

// UpdateResponseExportValues replaces the response_export_values attribute
// in azapi_resource "this" with the new set of computed paths.
func UpdateResponseExportValues(file *hclwrite.File, paths []string) error {
	for _, block := range file.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "azapi_resource" || labels[1] != "this" {
			continue
		}
		if len(paths) == 0 {
			block.Body().RemoveAttribute("response_export_values")
			return nil
		}
		block.Body().SetAttributeRaw("response_export_values", tokensForStringList(paths))
		return nil
	}
	return fmt.Errorf("no azapi_resource \"this\" block found")
}

// tokensForStringList creates HCL tokens for a list of string literals: ["a", "b", "c"]
func tokensForStringList(values []string) hclwrite.Tokens {
	ctyVals := make([]cty.Value, len(values))
	for i, v := range values {
		ctyVals[i] = cty.StringVal(v)
	}
	return hclwrite.TokensForValue(cty.ListVal(ctyVals))
}
