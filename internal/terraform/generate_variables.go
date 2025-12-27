package terraform

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
	"github.com/zclconf/go-cty/cty"
)

func generateVariables(schema *openapi3.Schema, supportsTags, supportsLocation, supportsIdentity bool, secrets []secretField, nameSchema *openapi3.Schema) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// Build a set of secret field variable names for quick lookup.
	secretVarNames := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		secretVarNames[secret.varName] = struct{}{}
	}

	appendVariable := func(name, description string, typeTokens hclwrite.Tokens) *hclwrite.Body {
		block := body.AppendNewBlock("variable", []string{name})
		varBody := block.Body()
		hclgen.SetDescriptionAttribute(varBody, strings.TrimSpace(description))
		varBody.SetAttributeRaw("type", typeTokens)
		return varBody
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

		varBody := appendVariable(tfName, "", tfType)

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

		// Mark secret fields as ephemeral
		if _, ok := secretVarNames[tfName]; ok {
			varBody.SetAttributeValue("ephemeral", cty.True)
		}

		// Generate validations for this variable
		generateValidations(varBody, tfName, propSchema, isRequired)
		if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") && len(propSchema.Properties) > 0 {
			if err := generateNestedObjectValidations(varBody, tfName, propSchema); err != nil {
				return nil, err
			}
		}

		return varBody, nil
	}

	nameVarBody := appendVariable("name", "The name of the resource.", hclwrite.TokensForIdentifier("string"))
	// The resource name constraints usually come from the operation path parameter schema (not the request body schema).
	// When available, apply them as validations to var.name.
	if nameSchema != nil {
		generateValidations(nameVarBody, "name", nameSchema, true)
	}
	body.AppendNewline()

	appendVariable("parent_id", "The parent resource ID for this resource.", hclwrite.TokensForIdentifier("string"))
	body.AppendNewline()

	if supportsLocation {
		locationDescription := "The location of the resource."
		locationType := hclwrite.TokensForIdentifier("string")
		appendVariable("location", locationDescription, locationType)
		body.AppendNewline()
	}

	if supportsTags {
		tagsBody := appendVariable("tags", "Tags to apply to the resource.", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")))
		tagsBody.SetAttributeValue("default", cty.NullVal(cty.Map(cty.String)))
		body.AppendNewline()
	}

	if supportsIdentity {
		managedIdentitiesBody := appendVariable(
			"managed_identities",
			"Managed identities configuration for this resource.",
			hclwrite.TokensForFunctionCall(
				"object",
				hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
					{Name: hclwrite.TokensForIdentifier("system_assigned"), Value: hclwrite.TokensForIdentifier("bool")},
					{Name: hclwrite.TokensForIdentifier("user_assigned_resource_ids"), Value: hclwrite.TokensForFunctionCall("list", hclwrite.TokensForIdentifier("string"))},
				}),
			),
		)
		managedIdentitiesBody.SetAttributeRaw(
			"default",
			hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
				{Name: hclwrite.TokensForIdentifier("system_assigned"), Value: hclwrite.TokensForIdentifier("false")},
				{Name: hclwrite.TokensForIdentifier("user_assigned_resource_ids"), Value: hclwrite.TokensForValue(cty.ListValEmpty(cty.String))},
			}),
		)
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
	if supportsIdentity {
		seenNames["managed_identities"] = struct{}{}
	}

	// Get effective properties and required (handling allOf)
	var keys []string
	var effectiveProps map[string]*openapi3.SchemaRef
	var effectiveRequired []string
	if schema != nil {
		var err error
		effectiveProps, err = openapi.GetEffectiveProperties(schema)
		if err != nil {
			return fmt.Errorf("getting effective properties: %w", err)
		}
		effectiveRequired, err = openapi.GetEffectiveRequired(schema)
		if err != nil {
			return fmt.Errorf("getting effective required: %w", err)
		}

		for k := range effectiveProps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	for i, name := range keys {
		prop := effectiveProps[name]
		if prop == nil || prop.Value == nil {
			continue
		}

		// Identity is handled via managed identity scaffolding in main.tf when supported.
		// When not supported, we avoid generating an input for identity, since many specs
		// only expose identity as read-only metadata.
		if name == "identity" {
			continue
		}
		if supportsTags && name == "tags" {
			continue
		}
		if supportsLocation && name == "location" {
			continue
		}
		propSchema := prop.Value

		if !isWritableProperty(propSchema) {
			continue
		}

		// Flatten the top-level "properties" bag into individual variables.
		if name == "properties" {
			propEffectiveProps, err := openapi.GetEffectiveProperties(propSchema)
			if err != nil {
				return fmt.Errorf("getting effective properties for 'properties': %w", err)
			}
			propEffectiveRequired, err := openapi.GetEffectiveRequired(propSchema)
			if err != nil {
				return fmt.Errorf("getting effective required for 'properties': %w", err)
			}

			if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") && len(propEffectiveProps) > 0 {
				var childKeys []string
				for ck := range propEffectiveProps {
					childKeys = append(childKeys, ck)
				}
				sort.Strings(childKeys)

				for _, ck := range childKeys {
					childRef := propEffectiveProps[ck]
					if childRef == nil || childRef.Value == nil {
						continue
					}
					childSchema := childRef.Value
					if !isWritableProperty(childSchema) {
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

					if _, err := appendSchemaVariable(tfName, ck, childSchema, propEffectiveRequired); err != nil {
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
		if _, err := appendSchemaVariable(tfName, name, propSchema, effectiveRequired); err != nil {
			return err
		}

		if i < len(keys)-1 {
			body.AppendNewline()
		}
	}

	// Add secret field variables (extracted from nested structures)
	secretBlockAdded := false
	for _, secret := range secrets {
		// If the variable already exists (e.g., flattened root properties), don't redeclare it.
		// The existing variable will already be marked ephemeral via secretVarNames.
		if _, exists := seenNames[secret.varName]; exists {
			continue
		}
		if !secretBlockAdded && len(keys) > 0 {
			body.AppendNewline()
			secretBlockAdded = true
		}

		secretVarBody := appendVariable(
			secret.varName,
			secret.schema.Description,
			mapType(secret.schema),
		)

		seenNames[secret.varName] = struct{}{}
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
		if _, exists := seenNames[versionVarName]; exists {
			return fmt.Errorf("terraform variable name collision: %q (from secret version var)", versionVarName)
		}
		versionBody := appendVariable(
			versionVarName,
			fmt.Sprintf("Version tracker for %s. Must be set when %s is provided.", secret.varName, secret.varName),
			hclwrite.TokensForIdentifier("number"),
		)
		seenNames[versionVarName] = struct{}{}

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
		// Get effective properties and required for allOf handling
		effectiveProps, err := openapi.GetEffectiveProperties(schema)
		if err != nil {
			// Errors indicate cycles or conflicts which should fail generation
			panic(fmt.Sprintf("failed to get effective properties: %v", err))
		}
		effectiveRequired, err := openapi.GetEffectiveRequired(schema)
		if err != nil {
			panic(fmt.Sprintf("failed to get effective required: %v", err))
		}

		if len(effectiveProps) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				valueType := mapType(schema.AdditionalProperties.Schema.Value)
				return hclwrite.TokensForFunctionCall("map", valueType)
			}
			return hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string"))
		}
		var attrs []hclwrite.ObjectAttrTokens

		// Sort properties
		var keys []string
		for k := range effectiveProps {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			prop := effectiveProps[k]
			if prop == nil || prop.Value == nil {
				continue
			}
			if !isWritableProperty(prop.Value) {
				continue
			}
			fieldType := mapType(prop.Value)

			// Check if optional
			isOptional := true
			if slices.Contains(effectiveRequired, k) {
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

	// Get effective properties for allOf handling
	effectiveProps, err := openapi.GetEffectiveProperties(schema)
	if err != nil {
		// Errors indicate cycles or conflicts which should fail generation
		panic(fmt.Sprintf("failed to get effective properties in buildNestedDescription: %v", err))
	}

	type keyPair struct {
		original string
		snake    string
	}
	var childKeys []keyPair
	for k := range effectiveProps {
		childKeys = append(childKeys, keyPair{original: k, snake: toSnakeCase(k)})
	}
	sort.Slice(childKeys, func(i, j int) bool {
		return childKeys[i].snake < childKeys[j].snake
	})

	for _, pair := range childKeys {
		k := pair.original
		childProp := effectiveProps[k]
		if childProp == nil || childProp.Value == nil {
			continue
		}
		val := childProp.Value

		if !isWritableProperty(val) {
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

		// Check if nested object has properties (considering allOf)
		nestedProps, err := openapi.GetEffectiveProperties(val)
		if err != nil {
			panic(fmt.Sprintf("failed to get effective properties for nested object: %v", err))
		}
		if isNested && len(nestedProps) > 0 {
			sb.WriteString(buildNestedDescription(val, indent+"  "))
		}
	}
	return sb.String()
}
