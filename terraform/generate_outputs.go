package terraform

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/zclconf/go-cty/cty"
)

// generateOutputs creates the outputs.tf file with AVM-compliant outputs.
// Always includes the mandatory AVM outputs: resource_id and name.
// Also includes outputs for computed/readOnly exported attributes when schema is available.
func buildOutputs(rs *schema.ResourceSchema) *hclwrite.File {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// AVM mandatory output: resource_id
	resourceID := body.AppendNewBlock("output", []string{"resource_id"})
	resourceIDBody := resourceID.Body()
	resourceIDBody.SetAttributeValue("description", cty.StringVal("The ID of the created resource."))
	resourceIDBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "id"))
	body.AppendNewline()

	// AVM mandatory output: name
	name := body.AppendNewBlock("output", []string{"name"})
	nameBody := name.Body()
	nameBody.SetAttributeValue("description", cty.StringVal("The name of the created resource."))
	nameBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "name"))
	body.AppendNewline()

	if rs != nil {
		exportPaths := extractComputedPaths(rs)
		usedNames := make(map[string]int)
		for _, exportPath := range exportPaths {
			outputName := outputNameForExportPath(exportPath)
			if outputName == "" {
				continue
			}
			if count, ok := usedNames[outputName]; ok {
				count++
				usedNames[outputName] = count
				outputName = fmt.Sprintf("%s_%d", outputName, count)
			} else {
				usedNames[outputName] = 1
			}

			out := body.AppendNewBlock("output", []string{outputName})
			outBody := out.Body()
			desc := "Computed value exported from the Azure API response."
			propForPath := propertyForExportPath(rs, exportPath)
			if propForPath != nil {
				if strings.TrimSpace(propForPath.Description) != "" {
					desc = strings.TrimSpace(propForPath.Description)
				}
			}
			outBody.SetAttributeValue("description", cty.StringVal(desc))

			segments := strings.Split(exportPath, ".")
			valueParts := make([]string, 0, 3+len(segments))
			valueParts = append(valueParts, "azapi_resource", "this", "output")
			valueParts = append(valueParts, segments...)
			expr := hclgen.TokensForTraversalOrIndex(valueParts...)
			outBody.SetAttributeRaw("value", hclwrite.TokensForFunctionCall("try", expr, defaultTokensForProperty(propForPath)))
			body.AppendNewline()
		}
	}

	return file
}

func generateOutputs(rs *schema.ResourceSchema, outputDir string) error {
	return hclgen.WriteFileToDir(outputDir, "outputs.tf", buildOutputs(rs))
}

// propertyForExportPath navigates the resource schema's property tree
// following a dot-separated export path.
func propertyForExportPath(rs *schema.ResourceSchema, exportPath string) *schema.Property {
	if rs == nil {
		return nil
	}
	exportPath = strings.TrimSpace(exportPath)
	if exportPath == "" {
		return nil
	}
	segments := strings.Split(exportPath, ".")
	return propertyForPathRecursive(rs.Properties, segments)
}

// propertyForPathRecursive walks through nested properties following path segments.
func propertyForPathRecursive(props map[string]*schema.Property, segments []string) *schema.Property {
	if len(props) == 0 || len(segments) == 0 {
		return nil
	}

	propName := segments[0]
	prop, ok := props[propName]
	if !ok || prop == nil {
		return nil
	}

	if len(segments) == 1 {
		return prop
	}

	// Navigate into children
	if prop.Children != nil {
		if found := propertyForPathRecursive(prop.Children, segments[1:]); found != nil {
			return found
		}
	}

	return nil
}

// defaultTokensForProperty returns the default fallback value tokens for a property type.
func defaultTokensForProperty(prop *schema.Property) hclwrite.Tokens {
	if prop == nil {
		return hclwrite.TokensForIdentifier("null")
	}

	switch prop.Type {
	case schema.TypeArray:
		return hclwrite.TokensForValue(cty.ListValEmpty(cty.DynamicPseudoType))
	case schema.TypeObject:
		return hclwrite.TokensForValue(cty.EmptyObjectVal)
	default:
		return hclwrite.TokensForIdentifier("null")
	}
}

func outputNameForExportPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return ""
	}
	if segments[0] == "properties" {
		segments = segments[1:]
	}
	nameSegments := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		nameSegments = append(nameSegments, naming.ToSnakeCase(seg))
	}
	if len(nameSegments) == 0 {
		return ""
	}
	outName := strings.Join(nameSegments, "_")
	// Do not generate outputs which overlap the AVM mandatory outputs or add
	// redundant aliases for azapi_resource.this.id.
	if outName == "name" || outName == "resource_id" || outName == "id" {
		return ""
	}
	return outName
}
