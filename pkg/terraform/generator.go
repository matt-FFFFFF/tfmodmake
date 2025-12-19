// Package terraform provides functions to generate Terraform variable and local definitions from OpenAPI schemas.
package terraform

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
func Generate(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool) error {
	hasSchema := schema != nil

	if err := generateTerraform(); err != nil {
		return err
	}
	if err := generateVariables(schema, supportsTags); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(schema, localName); err != nil {
			return err
		}
	}
	if err := generateMain(resourceType, apiVersion, localName, supportsTags, hasSchema); err != nil {
		return err
	}
	if err := generateOutputs(); err != nil {
		return err
	}
	return nil
}

func setExpressionAttribute(body *hclwrite.Body, name, expr string) error {
	tokens, err := tokensForExpression(expr)
	if err != nil {
		return fmt.Errorf("parsing expression %q: %w", expr, err)
	}
	body.SetAttributeRaw(name, tokens)
	return nil
}

func tokensForExpression(expr string) (hclwrite.Tokens, error) {
	content := fmt.Sprintf("attr = %s\n", expr)
	file, diag := hclwrite.ParseConfig([]byte(content), "expression.hcl", hcl.Pos{Line: 1, Column: 1})
	if diag.HasErrors() {
		return nil, diag
	}

	attr := file.Body().GetAttribute("attr")
	if attr == nil {
		return nil, fmt.Errorf("unable to parse expression")
	}

	return attr.Expr().BuildTokens(nil), nil
}

func writeHCLFile(path string, file *hclwrite.File) error {
	return os.WriteFile(path, file.Bytes(), 0o644)
}

func generateTerraform() error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	tfBlock := body.AppendNewBlock("terraform", nil)
	tfBody := tfBlock.Body()
	tfBody.SetAttributeValue("required_version", cty.StringVal("~> 1.12"))

	providers := tfBody.AppendNewBlock("required_providers", nil)
	azapi := providers.Body().AppendNewBlock("azapi", nil)
	azapiBody := azapi.Body()
	azapiBody.SetAttributeValue("source", cty.StringVal("azure/azapi"))
	azapiBody.SetAttributeValue("version", cty.StringVal("~> 2.7"))

	return writeHCLFile("terraform.tf", file)
}

func generateVariables(schema *openapi3.Schema, supportsTags bool) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	appendVariable := func(name, description, typeExpr string) (*hclwrite.Body, error) {
		block := body.AppendNewBlock("variable", []string{name})
		varBody := block.Body()
		varBody.SetAttributeValue("description", cty.StringVal(description))
		if err := setExpressionAttribute(varBody, "type", typeExpr); err != nil {
			return nil, err
		}
		return varBody, nil
	}

	if _, err := appendVariable("name", "The name of the resource.", "string"); err != nil {
		return err
	}

	if _, err := appendVariable("parent_id", "The parent resource ID for this resource.", "string"); err != nil {
		return err
	}

	if supportsTags {
		tagsBody, err := appendVariable("tags", "Tags to apply to the resource.", "map(string)")
		if err != nil {
			return err
		}
		if err := setExpressionAttribute(tagsBody, "default", "null"); err != nil {
			return err
		}
	}

	var keys []string
	if schema != nil {
		for k := range schema.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	for _, name := range keys {
		prop := schema.Properties[name]
		if prop == nil || prop.Value == nil {
			continue
		}
		propSchema := prop.Value

		if propSchema.ReadOnly {
			continue
		}

		tfName := toSnakeCase(name)
		tfType := mapType(propSchema)

		var nestedDocSchema *openapi3.Schema
		if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") {
			switch {
			case len(propSchema.Properties) > 0:
				nestedDocSchema = propSchema
			case propSchema.AdditionalProperties.Schema != nil && propSchema.AdditionalProperties.Schema.Value != nil:
				apSchema := propSchema.AdditionalProperties.Schema.Value
				if apSchema.Type != nil && slices.Contains(*apSchema.Type, "object") && len(apSchema.Properties) > 0 {
					nestedDocSchema = apSchema
				}
			}
		}
		isNestedObject := nestedDocSchema != nil

		varBody, err := appendVariable(tfName, "", tfType)
		if err != nil {
			return err
		}

		if isNestedObject {
			var sb strings.Builder
			desc := propSchema.Description
			if desc == "" {
				desc = fmt.Sprintf("The %s of the resource.", name)
			}
			sb.WriteString(desc)
			sb.WriteString("\n\n")

			if nestedDocSchema != propSchema {
				sb.WriteString("Map values:\n")
			}

			sb.WriteString(buildNestedDescription(nestedDocSchema, ""))

			varBody.SetAttributeValue("description", cty.StringVal(sb.String()))
		} else {
			description := propSchema.Description
			if description == "" {
				description = fmt.Sprintf("The %s of the resource.", name)
			}
			description = strings.ReplaceAll(description, "\n", " ")
			varBody.SetAttributeValue("description", cty.StringVal(description))
		}

		if err := setExpressionAttribute(varBody, "type", tfType); err != nil {
			return err
		}

		isRequired := slices.Contains(schema.Required, name)
		if !isRequired {
			if err := setExpressionAttribute(varBody, "default", "null"); err != nil {
				return err
			}
		}

		if len(propSchema.Enum) > 0 {
			var enumValues []string
			var enumValuesRaw []string
			for _, v := range propSchema.Enum {
				enumValues = append(enumValues, fmt.Sprintf("\"%v\"", v))
				enumValuesRaw = append(enumValuesRaw, fmt.Sprintf("%v", v))
			}
			enumStr := fmt.Sprintf("[%s]", strings.Join(enumValues, ", "))

			validation := varBody.AppendNewBlock("validation", nil)
			validationBody := validation.Body()

			if !isRequired {
				if err := setExpressionAttribute(validationBody, "condition", fmt.Sprintf("var.%s == null || contains(%s, var.%s)", tfName, enumStr, tfName)); err != nil {
					return err
				}
			} else {
				if err := setExpressionAttribute(validationBody, "condition", fmt.Sprintf("contains(%s, var.%s)", enumStr, tfName)); err != nil {
					return err
				}
			}

			validationBody.SetAttributeValue("error_message", cty.StringVal(fmt.Sprintf("%s must be one of: %s.", tfName, strings.Join(enumValuesRaw, ", "))))
		}
	}

	return writeHCLFile("variables.tf", file)
}

func generateLocals(schema *openapi3.Schema, localName string) error {
	if schema == nil {
		return nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	locals := body.AppendNewBlock("locals", nil)
	localBody := locals.Body()

	valueExpression := constructValue(schema, "var", true)
	if err := setExpressionAttribute(localBody, localName, valueExpression); err != nil {
		return err
	}

	return writeHCLFile("locals.tf", file)
}

func constructValue(schema *openapi3.Schema, accessPath string, isRoot bool) string {
	if schema.Type == nil {
		return accessPath
	}

	types := *schema.Type

	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				mappedValue := constructValue(schema.AdditionalProperties.Schema.Value, "value", false)
				return fmt.Sprintf("%s == null ? null : { for k, value in %s : k => %s }", accessPath, accessPath, mappedValue)
			}
			return accessPath // map(string) or free-form, passed as is
		}

		var fields []string
		var keys []string
		for k := range schema.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			prop := schema.Properties[k]
			if prop == nil || prop.Value == nil {
				continue
			}

			if prop.Value.ReadOnly {
				continue
			}

			snakeName := toSnakeCase(k)
			childAccess := fmt.Sprintf("%s.%s", accessPath, snakeName)
			if isRoot {
				// For root variables, access is var.snake_name
				childAccess = fmt.Sprintf("var.%s", snakeName)
			}

			childValue := constructValue(prop.Value, childAccess, false)
			fields = append(fields, fmt.Sprintf("%s = %s", k, childValue))
		}

		objStr := fmt.Sprintf("{\n%s\n}", strings.Join(fields, "\n"))

		if !isRoot {
			// If not root, handle null check
			return fmt.Sprintf("%s == null ? null : %s", accessPath, objStr)
		}
		return objStr
	}

	if slices.Contains(types, "array") {
		if schema.Items != nil && schema.Items.Value != nil {
			// [for x in accessPath : constructValue(items, x)]
			// We need a unique iterator variable name if nested?
			// Simple "item" might conflict if nested arrays?
			// Let's use a simple heuristic or just "item" since HCL scoping handles it?
			// Actually HCL `for` expressions create a new scope.

			childValue := constructValue(schema.Items.Value, "item", false)
			return fmt.Sprintf("%s == null ? null : [for item in %s : %s]", accessPath, accessPath, childValue)
		}
		return accessPath
	}

	return accessPath
}

func mapType(schema *openapi3.Schema) string {
	if schema.Type == nil {
		return "any"
	}

	types := *schema.Type

	if slices.Contains(types, "string") {
		return "string"
	}
	if slices.Contains(types, "integer") || slices.Contains(types, "number") {
		return "number"
	}
	if slices.Contains(types, "boolean") {
		return "bool"
	}
	if slices.Contains(types, "array") {
		elemType := "any"
		if schema.Items != nil && schema.Items.Value != nil {
			elemType = mapType(schema.Items.Value)
		}
		return fmt.Sprintf("list(%s)", elemType)
	}
	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				valueType := mapType(schema.AdditionalProperties.Schema.Value)
				return fmt.Sprintf("map(%s)", valueType)
			}
			return "map(string)" // Fallback for free-form objects
		}
		var fields []string

		// Sort properties
		var keys []string
		for k := range schema.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			prop := schema.Properties[k]
			if prop == nil || prop.Value == nil {
				continue
			}
			if prop.Value.ReadOnly {
				continue
			}
			fieldType := mapType(prop.Value)

			// Check if optional
			isOptional := true
			if slices.Contains(schema.Required, k) {
				isOptional = false
			}

			if isOptional {
				fields = append(fields, fmt.Sprintf("%s = optional(%s)", toSnakeCase(k), fieldType))
			} else {
				fields = append(fields, fmt.Sprintf("%s = %s", toSnakeCase(k), fieldType))
			}
		}
		return fmt.Sprintf("object({\n    %s\n  })", strings.Join(fields, "\n    "))
	}

	return "any"
}

func buildNestedDescription(schema *openapi3.Schema, indent string) string {
	var sb strings.Builder

	type keyPair struct {
		original string
		snake    string
	}
	var childKeys []keyPair
	for k := range schema.Properties {
		childKeys = append(childKeys, keyPair{original: k, snake: toSnakeCase(k)})
	}
	sort.Slice(childKeys, func(i, j int) bool {
		return childKeys[i].snake < childKeys[j].snake
	})

	for _, pair := range childKeys {
		k := pair.original
		childProp := schema.Properties[k]
		if childProp == nil || childProp.Value == nil {
			continue
		}
		val := childProp.Value

		if val.ReadOnly {
			continue
		}

		childDesc := val.Description
		if childDesc == "" {
			childDesc = fmt.Sprintf("The %s property.", k)
		}
		childDesc = strings.ReplaceAll(childDesc, "\n", " ")

		sb.WriteString(fmt.Sprintf("%s- `%s` - %s\n", indent, pair.snake, childDesc))

		isNested := false
		if val.Type != nil {
			if slices.Contains(*val.Type, "object") {
				isNested = true
			}
		}
		if isNested && len(val.Properties) > 0 {
			sb.WriteString(buildNestedDescription(val, indent+"  "))
		}
	}
	return sb.String()
}

func toSnakeCase(input string) string {
	var sb strings.Builder
	runes := []rune(input)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					sb.WriteRune('_')
				} else if unicode.IsUpper(prev) {
					// Check if we should split here
					// Standard rule: split if next is lower
					if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
						// Exception: if the lower part is just 's' (plural acronym), don't split.

						// Look ahead for lower case sequence
						j := i + 1
						for j < len(runes) && unicode.IsLower(runes[j]) {
							j++
						}
						lowerLen := j - (i + 1)

						if lowerLen > 1 {
							sb.WriteRune('_')
						} else if lowerLen == 1 && runes[i+1] != 's' {
							sb.WriteRune('_')
						}
					}
				}
			}
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	return sb.String()
}

// SupportsTags reports whether the schema includes a writable top-level "tags" property.
func SupportsTags(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}
	tagsProp, ok := schema.Properties["tags"]
	if !ok || tagsProp == nil || tagsProp.Value == nil {
		return false
	}
	if tagsProp.Value.ReadOnly {
		return false
	}
	return true
}

func generateMain(resourceType, apiVersion, localName string, supportsTags, hasSchema bool) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		apiVersion = "apiVersion"
	}
	resourceTypeWithAPIVersion := fmt.Sprintf("%s@%s", resourceType, apiVersion)

	resourceBlock := body.AppendNewBlock("resource", []string{"azapi_resource", "this"})
	resourceBody := resourceBlock.Body()
	resourceBody.SetAttributeValue("type", cty.StringVal(resourceTypeWithAPIVersion))
	if err := setExpressionAttribute(resourceBody, "name", "var.name"); err != nil {
		return err
	}
	if err := setExpressionAttribute(resourceBody, "parent_id", "var.parent_id"); err != nil {
		return err
	}

	bodyExpr := "{}"
	if hasSchema {
		bodyExpr = fmt.Sprintf("{\n  properties = local.%s\n}", localName)
	}
	if err := setExpressionAttribute(resourceBody, "body", bodyExpr); err != nil {
		return err
	}

	if supportsTags {
		if err := setExpressionAttribute(resourceBody, "tags", "var.tags"); err != nil {
			return err
		}
	}

	return writeHCLFile("main.tf", file)
}

func generateOutputs() error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	resourceID := body.AppendNewBlock("output", []string{"resource_id"})
	resourceIDBody := resourceID.Body()
	resourceIDBody.SetAttributeValue("description", cty.StringVal("The ID of the created resource."))
	if err := setExpressionAttribute(resourceIDBody, "value", "azapi_resource.this.id"); err != nil {
		return err
	}

	name := body.AppendNewBlock("output", []string{"name"})
	nameBody := name.Body()
	nameBody.SetAttributeValue("description", cty.StringVal("The name of the created resource."))
	if err := setExpressionAttribute(nameBody, "value", "azapi_resource.this.name"); err != nil {
		return err
	}

	return writeHCLFile("outputs.tf", file)
}
