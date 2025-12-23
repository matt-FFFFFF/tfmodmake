// Package terraform provides functions to generate Terraform variable and local definitions from OpenAPI schemas.
package terraform

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/pkg/hclgen"
	"github.com/zclconf/go-cty/cty"
)

// secretField represents a secret field detected in the schema.
type secretField struct {
	// path is the JSON path to the field, e.g., "properties.daprAIInstrumentationKey"
	path string
	// varName is the snake_case variable name, e.g., "dapr_ai_instrumentation_key"
	varName string
	// schema is the OpenAPI schema for this field
	schema *openapi3.Schema
}

// isSecretField checks if a schema property has x-ms-secret: true extension.
func isSecretField(schema *openapi3.Schema) bool {
	if schema == nil || schema.Extensions == nil {
		return false
	}
	if val, ok := schema.Extensions["x-ms-secret"]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false
}

// collectSecretFields traverses the schema and collects all fields marked with x-ms-secret.
func collectSecretFields(schema *openapi3.Schema, pathPrefix string) []secretField {
	var secrets []secretField
	if schema == nil {
		return secrets
	}

	var keys []string
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		prop := schema.Properties[name]
		if prop == nil || prop.Value == nil || prop.Value.ReadOnly {
			continue
		}

		propSchema := prop.Value
		currentPath := name
		if pathPrefix != "" {
			currentPath = pathPrefix + "." + name
		}

		if isSecretField(propSchema) {
			secrets = append(secrets, secretField{
				path:    currentPath,
				varName: toSnakeCase(name),
				schema:  propSchema,
			})
		}

		// Recursively check nested objects
		if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") && len(propSchema.Properties) > 0 {
			nested := collectSecretFields(propSchema, currentPath)
			secrets = append(secrets, nested...)
		}

		// Check array items for nested secrets
		if propSchema.Type != nil && slices.Contains(*propSchema.Type, "array") {
			if propSchema.Items != nil && propSchema.Items.Value != nil {
				itemSchema := propSchema.Items.Value
				if itemSchema.Type != nil && slices.Contains(*itemSchema.Type, "object") && len(itemSchema.Properties) > 0 {
					nested := collectSecretFields(itemSchema, currentPath+"[]")
					secrets = append(secrets, nested...)
				}
			}
		}
	}

	return secrets
}

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
func Generate(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool, supportsLocation bool) error {
	hasSchema := schema != nil

	// Collect secret fields from schema
	var secrets []secretField
	if hasSchema {
		secrets = collectSecretFields(schema, "")
	}

	if err := generateTerraform(); err != nil {
		return err
	}
	if err := generateVariables(schema, supportsTags, supportsLocation, secrets); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(schema, localName, secrets); err != nil {
			return err
		}
	}
	if err := generateMain(schema, resourceType, apiVersion, localName, supportsTags, supportsLocation, hasSchema, secrets); err != nil {
		return err
	}
	if err := generateOutputs(); err != nil {
		return err
	}
	return nil
}

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

func generateVariables(schema *openapi3.Schema, supportsTags, supportsLocation bool, secrets []secretField) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// Build a map of secret field variable names for quick lookup
	secretVarNames := make(map[string]bool)
	for _, secret := range secrets {
		secretVarNames[secret.varName] = true
	}

	appendVariable := func(name, description string, typeTokens hclwrite.Tokens) (*hclwrite.Body, error) {
		block := body.AppendNewBlock("variable", []string{name})
		varBody := block.Body()
		hclgen.SetDescriptionAttribute(varBody, strings.TrimSpace(description))
		varBody.SetAttributeRaw("type", typeTokens)
		return varBody, nil
	}

	if _, err := appendVariable("name", "The name of the resource.", hclwrite.TokensForIdentifier("string")); err != nil {
		return err
	}
	body.AppendNewline()

	if _, err := appendVariable("parent_id", "The parent resource ID for this resource.", hclwrite.TokensForIdentifier("string")); err != nil {
		return err
	}
	body.AppendNewline()

	if supportsLocation {
		locationDescription := "The location of the resource."
		locationType := hclwrite.TokensForIdentifier("string")

		_, err := appendVariable("location", locationDescription, locationType)
		if err != nil {
			return err
		}
		body.AppendNewline()
	}

	if supportsTags {
		tagsBody, err := appendVariable("tags", "Tags to apply to the resource.", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")))
		if err != nil {
			return err
		}
		tagsBody.SetAttributeValue("default", cty.NullVal(cty.Map(cty.String)))
		body.AppendNewline()
	}

	var keys []string
	if schema != nil {
		for k := range schema.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	for i, name := range keys {
		prop := schema.Properties[name]
		if prop == nil || prop.Value == nil {
			continue
		}
		if supportsTags && name == "tags" {
			continue
		}
		if supportsLocation && name == "location" {
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

			hclgen.SetDescriptionAttribute(varBody, sb.String())
		} else {
			description := propSchema.Description
			if description == "" {
				description = fmt.Sprintf("The %s of the resource.", name)
			}
			hclgen.SetDescriptionAttribute(varBody, description)
		}

		isRequired := slices.Contains(schema.Required, name)
		if !isRequired {
			varBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
		}

		// Mark secret fields as ephemeral
		if secretVarNames[tfName] {
			varBody.SetAttributeValue("ephemeral", cty.True)
		}

		if len(propSchema.Enum) > 0 {
			var enumValuesRaw []string
			var enumTokens []hclwrite.Tokens
			for _, v := range propSchema.Enum {
				enumValuesRaw = append(enumValuesRaw, fmt.Sprintf("%v", v))
				enumTokens = append(enumTokens, hclwrite.TokensForValue(cty.StringVal(fmt.Sprintf("%v", v))))
			}

			varRef := hclgen.TokensForTraversal("var", tfName)
			enumList := hclwrite.TokensForTuple(enumTokens)
			containsCall := hclwrite.TokensForFunctionCall("contains", enumList, varRef)

			var condition hclwrite.Tokens
			if !isRequired {
				condition = append(condition, varRef...)
				condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte(" == ")})
				condition = append(condition, hclwrite.TokensForIdentifier("null")...)
				condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
				condition = append(condition, containsCall...)
			} else {
				condition = containsCall
			}

			validation := varBody.AppendNewBlock("validation", nil)
			validationBody := validation.Body()
			validationBody.SetAttributeRaw("condition", condition)
			validationBody.SetAttributeValue("error_message", cty.StringVal(fmt.Sprintf("%s must be one of: %s.", tfName, strings.Join(enumValuesRaw, ", "))))
		}

		if i < len(keys)-1 {
			body.AppendNewline()
		}
	}

	// Add secret field variables (extracted from nested structures)
	for i, secret := range secrets {
		if i == 0 && len(keys) > 0 {
			body.AppendNewline()
		}
		
		secretVarBody, err := appendVariable(
			secret.varName,
			secret.schema.Description,
			mapType(secret.schema),
		)
		if err != nil {
			return err
		}
		
		secretVarBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
		secretVarBody.SetAttributeValue("ephemeral", cty.True)
		
		body.AppendNewline()
	}

	// Add secret version variables
	for i, secret := range secrets {
		if i == 0 && len(keys) > 0 {
			body.AppendNewline()
		}
		versionVarName := secret.varName + "_version"
		versionBody, err := appendVariable(
			versionVarName,
			fmt.Sprintf("Version tracker for %s. Must be set when %s is provided.", secret.varName, secret.varName),
			hclwrite.TokensForIdentifier("number"),
		)
		if err != nil {
			return err
		}

		versionBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))

		// Add validation that version must be set when secret is set
		validation := versionBody.AppendNewBlock("validation", nil)
		validationBody := validation.Body()

		// Build condition: var.secret == null || var.secret_version != null
		secretVarRef := hclgen.TokensForTraversal("var", secret.varName)
		versionVarRef := hclgen.TokensForTraversal("var", versionVarName)

		var condition hclwrite.Tokens
		condition = append(condition, secretVarRef...)
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte(" == ")})
		condition = append(condition, hclwrite.TokensForIdentifier("null")...)
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
		condition = append(condition, versionVarRef...)
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte(" != ")})
		condition = append(condition, hclwrite.TokensForIdentifier("null")...)

		validationBody.SetAttributeRaw("condition", condition)
		validationBody.SetAttributeValue(
			"error_message",
			cty.StringVal(fmt.Sprintf("When %s is set, %s must also be set.", secret.varName, versionVarName)),
		)

		if i < len(secrets)-1 {
			body.AppendNewline()
		}
	}

	return hclgen.WriteFile("variables.tf", file)
}

func generateLocals(schema *openapi3.Schema, localName string, secrets []secretField) error {
	if schema == nil {
		return nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	locals := body.AppendNewBlock("locals", nil)
	localBody := locals.Body()

	valueExpression := constructValue(schema, hclwrite.TokensForIdentifier("var"), true, secrets)
	localBody.SetAttributeRaw(localName, valueExpression)

	return hclgen.WriteFile("locals.tf", file)
}

func constructValue(schema *openapi3.Schema, accessPath hclwrite.Tokens, isRoot bool, secrets []secretField) hclwrite.Tokens {
	// Build a set of secret field names for quick lookup
	secretNames := make(map[string]bool)
	for _, secret := range secrets {
		// Extract the last component of the path for direct property matching
		parts := strings.Split(secret.path, ".")
		if len(parts) > 0 {
			secretNames[parts[len(parts)-1]] = true
		}
	}

	if schema.Type == nil {
		return accessPath
	}

	types := *schema.Type

	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				mappedValue := constructValue(schema.AdditionalProperties.Schema.Value, hclwrite.TokensForIdentifier("value"), false, secrets)

				var tokens hclwrite.Tokens
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("for")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("k")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("value")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in")})
				tokens = append(tokens, accessPath...)
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("k")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenFatArrow, Bytes: []byte("=>")})
				tokens = append(tokens, mappedValue...)
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})

				if !isRoot {
					return hclgen.NullEqualityTernary(accessPath, tokens)
				}
				return tokens
			}
			return accessPath // map(string) or free-form, passed as is
		}

		var attrs []hclwrite.ObjectAttrTokens
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

			// Skip secret fields
			if secretNames[k] {
				continue
			}

			snakeName := toSnakeCase(k)
			var childAccess hclwrite.Tokens
			childAccess = append(childAccess, accessPath...)
			childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
			childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

			childValue := constructValue(prop.Value, childAccess, false, secrets)
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForIdentifier(k),
				Value: childValue,
			})
		}

		objTokens := hclwrite.TokensForObject(attrs)

		if !isRoot {
			return hclgen.NullEqualityTernary(accessPath, objTokens)
		}
		return objTokens
	}

	if slices.Contains(types, "array") {
		if schema.Items != nil && schema.Items.Value != nil {
			childValue := constructValue(schema.Items.Value, hclwrite.TokensForIdentifier("item"), false, secrets)

			var tokens hclwrite.Tokens
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("for")})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("item")})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in")})
			tokens = append(tokens, accessPath...)
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
			tokens = append(tokens, childValue...)
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})

			if !isRoot {
				return hclgen.NullEqualityTernary(accessPath, tokens)
			}
			return tokens
		}
		return accessPath
	}

	return accessPath
}

func mapType(schema *openapi3.Schema) hclwrite.Tokens {
	if schema.Type == nil {
		return hclwrite.TokensForIdentifier("any")
	}

	types := *schema.Type

	if slices.Contains(types, "string") {
		return hclwrite.TokensForIdentifier("string")
	}
	if slices.Contains(types, "integer") || slices.Contains(types, "number") {
		return hclwrite.TokensForIdentifier("number")
	}
	if slices.Contains(types, "boolean") {
		return hclwrite.TokensForIdentifier("bool")
	}
	if slices.Contains(types, "array") {
		elemType := hclwrite.TokensForIdentifier("any")
		if schema.Items != nil && schema.Items.Value != nil {
			elemType = mapType(schema.Items.Value)
		}
		return hclwrite.TokensForFunctionCall("list", elemType)
	}
	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				valueType := mapType(schema.AdditionalProperties.Schema.Value)
				return hclwrite.TokensForFunctionCall("map", valueType)
			}
			return hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string"))
		}
		var attrs []hclwrite.ObjectAttrTokens

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
				fieldType = hclwrite.TokensForFunctionCall("optional", fieldType)
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForIdentifier(toSnakeCase(k)),
				Value: fieldType,
			})
		}
		return hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject(attrs))
	}

	return hclwrite.TokensForIdentifier("any")
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
			if i == 0 {
				sb.WriteRune(unicode.ToLower(r))
				continue
			}
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				sb.WriteRune('_')
			}
			if unicode.IsUpper(prev) {
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
					}
					if lowerLen == 1 && runes[i+1] != 's' {
						sb.WriteRune('_')
					}
				}
			}
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	return sb.String()
}

// SupportsTags reports whether the schema includes a writable "tags" property, following allOf inheritance.
func SupportsTags(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "tags")
}

// SupportsLocation reports whether the schema includes a writable "location" property, following allOf inheritance.
func SupportsLocation(schema *openapi3.Schema) bool {
	return hasWritableProperty(schema, "location")
}

func hasWritableProperty(schema *openapi3.Schema, path string) bool {
	if schema == nil || path == "" {
		return false
	}
	segments := strings.Split(path, ".")
	return hasWritablePropertyRecursive(schema, segments, make(map[*openapi3.Schema]struct{}))
}

func hasWritablePropertyRecursive(schema *openapi3.Schema, segments []string, visited map[*openapi3.Schema]struct{}) bool {
	if schema == nil || len(segments) == 0 {
		return false
	}
	if _, seen := visited[schema]; seen {
		return false
	}
	visited[schema] = struct{}{}

	propName := segments[0]
	propRef, ok := schema.Properties[propName]
	if ok && propRef != nil && propRef.Value != nil && !propRef.Value.ReadOnly {
		if len(segments) == 1 {
			return true
		}
		if hasWritablePropertyRecursive(propRef.Value, segments[1:], visited) {
			return true
		}
	}

	for _, ref := range schema.AllOf {
		if ref == nil || ref.Value == nil {
			continue
		}
		if hasWritablePropertyRecursive(ref.Value, segments, visited) {
			return true
		}
	}

	return false
}

func cleanTypeString(typeStr string) string {
	segments := strings.Split(typeStr, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			continue
		}
		cleaned = append(cleaned, segment)
	}
	return strings.Join(cleaned, "/")
}

func generateMain(schema *openapi3.Schema, resourceType, apiVersion, localName string, supportsTags, supportsLocation, hasSchema bool, secrets []secretField) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		apiVersion = "apiVersion"
	}
	resourceTypeWithAPIVersion := fmt.Sprintf("%s@%s", cleanTypeString(resourceType), apiVersion)

	resourceBlock := body.AppendNewBlock("resource", []string{"azapi_resource", "this"})
	resourceBody := resourceBlock.Body()
	resourceBody.SetAttributeValue("type", cty.StringVal(resourceTypeWithAPIVersion))
	resourceBody.SetAttributeRaw("name", hclgen.TokensForTraversal("var", "name"))
	resourceBody.SetAttributeRaw("parent_id", hclgen.TokensForTraversal("var", "parent_id"))

	if supportsLocation {
		resourceBody.SetAttributeRaw("location", hclgen.TokensForTraversal("var", "location"))
	}

	resourceBody.SetAttributeValue("body", cty.EmptyObjectVal)
	if hasSchema {
		resourceBody.SetAttributeRaw("body", hclwrite.TokensForObject(
			[]hclwrite.ObjectAttrTokens{
				{
					Name:  hclwrite.TokensForIdentifier("properties"),
					Value: hclgen.TokensForTraversal("local", localName),
				},
			},
		))
	}

	// Add sensitive_body if there are secrets
	if len(secrets) > 0 {
		var sensitiveAttrs []hclwrite.ObjectAttrTokens
		for _, secret := range secrets {
			// Build the JSON path for the secret (e.g., "properties.connectionString")
			jsonPath := secret.path
			
			// Create attribute tokens for secret: "properties.fieldName" = var.field_name
			sensitiveAttrs = append(sensitiveAttrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForValue(cty.StringVal(jsonPath)),
				Value: hclgen.TokensForTraversal("var", secret.varName),
			})
		}
		resourceBody.SetAttributeRaw("sensitive_body", hclwrite.TokensForObject(sensitiveAttrs))

		// Add sensitive_body_version map
		var versionAttrs []hclwrite.ObjectAttrTokens
		for _, secret := range secrets {
			versionVarName := secret.varName + "_version"
			versionAttrs = append(versionAttrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForValue(cty.StringVal(secret.path)),
				Value: hclgen.TokensForTraversal("var", versionVarName),
			})
		}
		resourceBody.SetAttributeRaw("sensitive_body_version", hclwrite.TokensForObject(versionAttrs))
	}

	if supportsTags {
		resourceBody.SetAttributeRaw("tags", hclgen.TokensForTraversal("var", "tags"))
	}

	resourceBody.SetAttributeValue("response_export_values", cty.ListValEmpty(cty.String))

	return hclgen.WriteFile("main.tf", file)
}

func generateOutputs() error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	resourceID := body.AppendNewBlock("output", []string{"resource_id"})
	resourceIDBody := resourceID.Body()
	resourceIDBody.SetAttributeValue("description", cty.StringVal("The ID of the created resource."))
	resourceIDBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "id"))

	name := body.AppendNewBlock("output", []string{"name"})
	nameBody := name.Body()
	nameBody.SetAttributeValue("description", cty.StringVal("The name of the created resource."))
	nameBody.SetAttributeRaw("value", hclgen.TokensForTraversal("azapi_resource", "this", "name"))

	return hclgen.WriteFile("outputs.tf", file)
}
