package terraform

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

// generateOutputs creates the outputs.tf file with AVM-compliant outputs.
// Always includes the mandatory AVM outputs: resource_id and name.
// Also includes outputs for computed/readOnly exported attributes when schema is available.
func generateOutputs(schema *openapi3.Schema) error {
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

	if schema != nil {
		exportPaths := extractComputedPaths(schema)
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
			propSchema := schemaForExportPath(schema, exportPath)
			if propSchema != nil {
				if strings.TrimSpace(propSchema.Description) != "" {
					desc = strings.TrimSpace(propSchema.Description)
				}
			}
			outBody.SetAttributeValue("description", cty.StringVal(desc))

			segments := strings.Split(exportPath, ".")
			valueParts := make([]string, 0, 3+len(segments))
			valueParts = append(valueParts, "azapi_resource", "this", "output")
			valueParts = append(valueParts, segments...)
			expr := hclgen.TokensForTraversalOrIndex(valueParts...)
			outBody.SetAttributeRaw("value", hclwrite.TokensForFunctionCall("try", expr, defaultTokensForSchema(propSchema)))
			body.AppendNewline()
		}
	}

	return hclgen.WriteFile("outputs.tf", file)
}

func schemaForExportPath(schema *openapi3.Schema, exportPath string) *openapi3.Schema {
	if schema == nil {
		return nil
	}
	exportPath = strings.TrimSpace(exportPath)
	if exportPath == "" {
		return nil
	}
	segments := strings.Split(exportPath, ".")
	return schemaForPathRecursive(schema, segments, make(map[*openapi3.Schema]struct{}))
}

func schemaForPathRecursive(schema *openapi3.Schema, segments []string, visited map[*openapi3.Schema]struct{}) *openapi3.Schema {
	if schema == nil || len(segments) == 0 {
		return nil
	}
	if _, seen := visited[schema]; seen {
		return nil
	}
	visited[schema] = struct{}{}
	defer delete(visited, schema)

	propName := segments[0]
	if propRef, ok := schema.Properties[propName]; ok && propRef != nil && propRef.Value != nil {
		if len(segments) == 1 {
			return propRef.Value
		}
		if found := schemaForPathRecursive(propRef.Value, segments[1:], visited); found != nil {
			return found
		}
	}

	for _, ref := range schema.AllOf {
		if ref == nil || ref.Value == nil {
			continue
		}
		if found := schemaForPathRecursive(ref.Value, segments, visited); found != nil {
			return found
		}
	}

	return nil
}

func defaultTokensForSchema(schema *openapi3.Schema) hclwrite.Tokens {
	if schema == nil || schema.Type == nil {
		return hclwrite.TokensForIdentifier("null")
	}

	for _, typ := range *schema.Type {
		if typ == "array" {
			return hclwrite.TokensForValue(cty.ListValEmpty(cty.DynamicPseudoType))
		}
		if typ == "object" {
			return hclwrite.TokensForValue(cty.EmptyObjectVal)
		}
	}

	return hclwrite.TokensForIdentifier("null")
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
		nameSegments = append(nameSegments, toSnakeCase(seg))
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
