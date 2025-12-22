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

// Generate generates variables.tf, locals.tf, main.tf, and outputs.tf based on the schema.
func Generate(schema *openapi3.Schema, resourceType string, localName string, apiVersion string, supportsTags bool, supportsLocation bool) error {
	hasSchema := schema != nil

	if err := generateTerraform(); err != nil {
		return err
	}
	if err := generateVariables(schema, supportsTags, supportsLocation); err != nil {
		return err
	}
	if hasSchema {
		if err := generateLocals(schema, localName); err != nil {
			return err
		}
	}
	if err := generateMain(schema, resourceType, apiVersion, localName, supportsTags, supportsLocation, hasSchema); err != nil {
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

func generateVariables(schema *openapi3.Schema, supportsTags, supportsLocation bool) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	appendVariable := func(name, description string, typeTokens hclwrite.Tokens) (*hclwrite.Body, error) {
		block := body.AppendNewBlock("variable", []string{name})
		varBody := block.Body()
		hclgen.SetDescriptionAttribute(varBody, strings.TrimSpace(description))
		varBody.SetAttributeRaw("type", typeTokens)
		return varBody, nil
	}

	appendSchemaVariable := func(tfName, originalName string, propSchema *openapi3.Schema, required []string) (*hclwrite.Body, error) {
		if propSchema == nil {
			return nil, nil
		}

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
			return nil, err
		}

		if isNestedObject {
			var sb strings.Builder
			desc := propSchema.Description
			if desc == "" {
				if originalName != "" {
					desc = fmt.Sprintf("The %s of the resource.", originalName)
				} else {
					desc = fmt.Sprintf("The %s of the resource.", tfName)
				}
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
				if originalName != "" {
					description = fmt.Sprintf("The %s of the resource.", originalName)
				} else {
					description = fmt.Sprintf("The %s of the resource.", tfName)
				}
			}
			hclgen.SetDescriptionAttribute(varBody, description)
		}

		isRequired := slices.Contains(required, originalName)
		if !isRequired {
			varBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
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

	seenNames := map[string]struct{}{
		"name":      {},
		"parent_id": {},
	}
	if supportsLocation {
		seenNames["location"] = struct{}{}
	}
	if supportsTags {
		seenNames["tags"] = struct{}{}
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

		// Flatten the top-level "properties" bag into individual variables.
		if name == "properties" {
			if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") && len(propSchema.Properties) > 0 {
				var childKeys []string
				for ck := range propSchema.Properties {
					childKeys = append(childKeys, ck)
				}
				sort.Strings(childKeys)

				for _, ck := range childKeys {
					childRef := propSchema.Properties[ck]
					if childRef == nil || childRef.Value == nil {
						continue
					}
					childSchema := childRef.Value
					if childSchema.ReadOnly {
						continue
					}
					tfName := toSnakeCase(ck)
					if tfName == "" {
						return fmt.Errorf("could not derive terraform variable name for properties.%s", ck)
					}
					if _, exists := seenNames[tfName]; exists {
						return fmt.Errorf("terraform variable name collision: %q (from properties.%s)", tfName, ck)
					}
					seenNames[tfName] = struct{}{}

					if _, err := appendSchemaVariable(tfName, ck, childSchema, propSchema.Required); err != nil {
						return err
					}
					body.AppendNewline()
				}
				continue
			}
			// If "properties" isn't a concrete object, fall back to the old behavior.
		}

		tfName := toSnakeCase(name)
		if tfName == "" {
			return fmt.Errorf("could not derive terraform variable name for %s", name)
		}
		if _, exists := seenNames[tfName]; exists {
			return fmt.Errorf("terraform variable name collision: %q (from %s)", tfName, name)
		}
		seenNames[tfName] = struct{}{}
		if _, err := appendSchemaVariable(tfName, name, propSchema, schema.Required); err != nil {
			return err
		}

		if i < len(keys)-1 {
			body.AppendNewline()
		}
	}

	return hclgen.WriteFile("variables.tf", file)
}

func generateLocals(schema *openapi3.Schema, localName string) error {
	if schema == nil {
		return nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	locals := body.AppendNewBlock("locals", nil)
	localBody := locals.Body()

	valueExpression := constructValue(schema, hclwrite.TokensForIdentifier("var"), true)
	localBody.SetAttributeRaw(localName, valueExpression)

	return hclgen.WriteFile("locals.tf", file)
}

func isHCLIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func tokensForObjectKey(key string) hclwrite.Tokens {
	if isHCLIdentifier(key) {
		return hclwrite.TokensForIdentifier(key)
	}
	return hclwrite.TokensForValue(cty.StringVal(key))
}

func constructFlattenedRootPropertiesValue(schema *openapi3.Schema, accessPath hclwrite.Tokens) hclwrite.Tokens {
	// schema represents the OpenAPI schema at root.properties.
	// The Terraform variables are flattened to var.<child> rather than var.properties.<child>.

	if schema == nil {
		return hclwrite.TokensForIdentifier("null")
	}

	var attrs []hclwrite.ObjectAttrTokens
	var keys []string
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var allNullConditions []hclwrite.Tokens
	for _, k := range keys {
		prop := schema.Properties[k]
		if prop == nil || prop.Value == nil {
			continue
		}
		if prop.Value.ReadOnly {
			continue
		}

		snakeName := toSnakeCase(k)
		var childAccess hclwrite.Tokens
		childAccess = append(childAccess, accessPath...)
		childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
		childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

		// Used to null-out the whole properties object when nothing is set.
		// We only do this when the properties schema doesn't declare required children.
		condition := append(hclwrite.Tokens(nil), childAccess...)
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte(" == ")})
		condition = append(condition, hclwrite.TokensForIdentifier("null")...)
		allNullConditions = append(allNullConditions, condition)

		childValue := constructValue(prop.Value, childAccess, false)
		attrs = append(attrs, hclwrite.ObjectAttrTokens{
			Name:  tokensForObjectKey(k),
			Value: childValue,
		})
	}

	objTokens := hclwrite.TokensForObject(attrs)

	if len(schema.Required) > 0 {
		return objTokens
	}

	if len(allNullConditions) == 0 {
		return hclwrite.TokensForIdentifier("null")
	}

	// Build: (var.a == null && var.b == null && ...) ? null : { ... }
	var condition hclwrite.Tokens
	for i, c := range allNullConditions {
		if i > 0 {
			condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenAnd, Bytes: []byte(" && ")})
		}
		condition = append(condition, c...)
	}

	var tokens hclwrite.Tokens
	tokens = append(tokens, condition...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte(" ? ")})
	tokens = append(tokens, hclwrite.TokensForIdentifier("null")...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(" : ")})
	tokens = append(tokens, objTokens...)
	return tokens
}

func constructValue(schema *openapi3.Schema, accessPath hclwrite.Tokens, isRoot bool) hclwrite.Tokens {
	if schema.Type == nil {
		return accessPath
	}

	types := *schema.Type

	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				mappedValue := constructValue(schema.AdditionalProperties.Schema.Value, hclwrite.TokensForIdentifier("value"), false)

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

			// Flatten the top-level "properties" bag into separate variables.
			if isRoot && k == "properties" && prop.Value.Type != nil && slices.Contains(*prop.Value.Type, "object") && len(prop.Value.Properties) > 0 {
				childValue := constructFlattenedRootPropertiesValue(prop.Value, accessPath)
				attrs = append(attrs, hclwrite.ObjectAttrTokens{
					Name:  tokensForObjectKey(k),
					Value: childValue,
				})
				continue
			}

			snakeName := toSnakeCase(k)
			var childAccess hclwrite.Tokens
			childAccess = append(childAccess, accessPath...)
			childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
			childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

			childValue := constructValue(prop.Value, childAccess, false)
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  tokensForObjectKey(k),
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
			childValue := constructValue(schema.Items.Value, hclwrite.TokensForIdentifier("item"), false)

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

	prevWasUnderscore := false
	wroteAny := false

	isAlnum := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	prevAlnum := func(i int) (rune, bool) {
		for j := i - 1; j >= 0; j-- {
			if isAlnum(runes[j]) {
				return runes[j], true
			}
		}
		return 0, false
	}
	nextAlnum := func(i int) (rune, bool) {
		for j := i + 1; j < len(runes); j++ {
			if isAlnum(runes[j]) {
				return runes[j], true
			}
		}
		return 0, false
	}

	for i, r := range runes {
		// Treat non-alphanumerics (e.g. '-', '.', spaces) as separators.
		if !isAlnum(r) {
			if wroteAny && !prevWasUnderscore {
				sb.WriteRune('_')
				prevWasUnderscore = true
			}
			continue
		}

		if unicode.IsUpper(r) {
			if p, ok := prevAlnum(i); ok {
				if (unicode.IsLower(p) || unicode.IsDigit(p)) && !prevWasUnderscore {
					sb.WriteRune('_')
				}
				if unicode.IsUpper(p) {
					// Split acronyms when the next alnum is lower (HTTPClient -> http_client)
					if n, ok := nextAlnum(i); ok && unicode.IsLower(n) {
						// Look ahead for a lower-case sequence length
						j := i + 1
						for j < len(runes) {
							if !isAlnum(runes[j]) {
								j++
								continue
							}
							if !unicode.IsLower(runes[j]) {
								break
							}
							j++
						}
						lowerLen := j - (i + 1)

						if lowerLen > 1 && !prevWasUnderscore {
							sb.WriteRune('_')
						}
						if lowerLen == 1 && n != 's' && !prevWasUnderscore {
							sb.WriteRune('_')
						}
					}
				}
			}
		}

		sb.WriteRune(unicode.ToLower(r))
		wroteAny = true
		prevWasUnderscore = false
	}

	out := strings.Trim(sb.String(), "_")
	if out == "" {
		return out
	}
	if len(out) > 0 && out[0] >= '0' && out[0] <= '9' {
		out = "field_" + out
	}
	return out
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

func generateMain(schema *openapi3.Schema, resourceType, apiVersion, localName string, supportsTags, supportsLocation, hasSchema bool) error {
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
		// If the schema already has a top-level "properties" field, we are generating the full resource body.
		// Otherwise, we assume the schema itself represents the properties object (e.g. -root properties).
		_, hasTopLevelProperties := schema.Properties["properties"]
		if hasTopLevelProperties {
			resourceBody.SetAttributeRaw("body", hclgen.TokensForTraversal("local", localName))
		} else {
			resourceBody.SetAttributeRaw("body", hclwrite.TokensForObject(
				[]hclwrite.ObjectAttrTokens{
					{
						Name:  hclwrite.TokensForIdentifier("properties"),
						Value: hclgen.TokensForTraversal("local", localName),
					},
				},
			))
		}
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
