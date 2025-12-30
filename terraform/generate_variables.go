package terraform

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
	"github.com/zclconf/go-cty/cty"
)

func generateVariables(schema *openapi3.Schema, supportsTags, supportsLocation, supportsIdentity bool, secrets []secretField, nameSchema *openapi3.Schema, caps openapi.InterfaceCapabilities, moduleNamePrefix string, outputDir string) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	arrayItemsContainSecret := func(schema *openapi3.Schema) (bool, error) {
		if schema == nil || schema.Type == nil {
			return false, nil
		}
		if !slices.Contains(*schema.Type, "array") {
			return false, nil
		}
		if schema.Items == nil || schema.Items.Value == nil {
			return false, nil
		}
		itemSchema := schema.Items.Value
		if itemSchema.Type == nil || !slices.Contains(*itemSchema.Type, "object") {
			return false, nil
		}
		props, err := openapi.GetEffectiveProperties(itemSchema)
		if err != nil {
			return false, fmt.Errorf("getting effective properties for array item schema: %w", err)
		}
		for _, prop := range props {
			if prop == nil || prop.Value == nil {
				continue
			}
			if !isWritableProperty(prop.Value) {
				continue
			}
			if isSecretField(prop.Value) {
				return true, nil
			}
		}
		return false, nil
	}

	appendTFLintIgnoreUnused := func() {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			&hclwrite.Token{Type: hclsyntax.TokenComment, Bytes: []byte("# tflint-ignore: terraform_unused_declarations")},
			&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		})
	}

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

		tfType, err := mapType(propSchema)
		if err != nil {
			return nil, err
		}

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

			nested, err := buildNestedDescription(nestedDocSchema, "")
			if err != nil {
				return nil, err
			}
			sb.WriteString(nested)
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

		// If this is an array of objects that contains secret fields in its items,
		// mark the whole variable as ephemeral. We currently don't generate array-aware
		// sensitive_body, so this prevents secrets from persisting in state.
		hasSecrets, err := arrayItemsContainSecret(propSchema)
		if err != nil {
			return nil, err
		}
		if hasSecrets {
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

	// AVM standard variables (declared up-front; may be unused depending on resource capabilities)
	// location
	appendVariable("location", "The location of the resource.", hclwrite.TokensForIdentifier("string"))
	body.AppendNewline()

	// tags (only when the resource supports tags)
	if supportsTags {
		appendTFLintIgnoreUnused()
		tagsBody := appendVariable("tags", "(Optional) Tags of the resource.", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")))
		tagsBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
		body.AppendNewline()
	}

	// managed_identities (only when the resource supports configuring identity)
	if supportsIdentity {
		appendTFLintIgnoreUnused()
		miBody := appendVariable(
			"managed_identities",
			"Controls the Managed Identity configuration on this resource.",
			hclwrite.TokensForFunctionCall(
				"object",
				hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
					{Name: hclwrite.TokensForIdentifier("system_assigned"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("bool"), hclwrite.TokensForIdentifier("false"))},
					{Name: hclwrite.TokensForIdentifier("user_assigned_resource_ids"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("set", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.ListValEmpty(cty.String)))},
				}),
			),
		)
		miBody.SetAttributeRaw("default", hclwrite.TokensForObject(nil))
		miBody.SetAttributeValue("nullable", cty.False)
		body.AppendNewline()
	}

	reservedNames := map[string]struct{}{
		"name":                 {},
		"parent_id":            {},
		"location":             {},
		"customer_managed_key": {},
		"diagnostic_settings":  {},
		"enable_telemetry":     {},
		"role_assignments":     {},
		"lock":                 {},
		"private_endpoints":    {},
		"private_endpoints_manage_dns_zone_group": {},
	}
	if supportsTags {
		reservedNames["tags"] = struct{}{}
	}
	if supportsIdentity {
		reservedNames["managed_identities"] = struct{}{}
	}

	seenNames := map[string]struct{}{}
	for k := range reservedNames {
		seenNames[k] = struct{}{}
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

		tfName := naming.ToSnakeCase(name)
		if tfName == "" {
			return fmt.Errorf("could not derive terraform variable name for %s", name)
		}
		if _, reserved := reservedNames[tfName]; reserved {
			continue
		}
		// Rename variables that conflict with Terraform module meta-arguments
		if moduleNamePrefix != "" && tfName == "version" {
			tfName = moduleNamePrefix + "_version"
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

		tfType, err := mapType(secret.schema)
		if err != nil {
			return err
		}
		secretVarBody := appendVariable(
			secret.varName,
			secret.schema.Description,
			tfType,
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

	// Add AVM interface variables
	// Only generate these when capabilities indicate support from REST spec
	if len(secrets) > 0 || len(keys) > 0 {
		body.AppendNewline()
	}

	// customer_managed_key (only if supported based on encryption properties in schema)
	emitCustomerManagedKeyVar(body, caps, appendVariable, appendTFLintIgnoreUnused)

	// enable_telemetry (always included for AVM compliance)
	emitEnableTelemetryVar(body, appendVariable)

	// diagnostic_settings (only if swagger indicates support)
	emitDiagnosticSettingsVar(body, caps, appendVariable)

	// role_assignments (ARM-level capability, not detectable from specs - omitted for child modules)
	// Note: For root modules, this could be included by default, but for consistency we omit unless detected
	// Users can add this manually or via a future helper command
	_ = caps // Explicitly show we're aware of capabilities but choosing not to generate role_assignments

	// lock (ARM-level capability, not detectable from specs - omitted for child modules)
	// Note: For root modules, this could be included by default, but for consistency we omit unless detected
	// Users can add this manually or via a future helper command

	// private_endpoints (only if swagger indicates Private Link/Private Endpoint support)
	emitPrivateEndpointsVars(body, caps, appendVariable)

	return hclgen.WriteFileToDir(outputDir, "variables.tf", file)
}

func mapType(schema *openapi3.Schema) (hclwrite.Tokens, error) {
	if schema.Type == nil {
		return hclwrite.TokensForIdentifier("any"), nil
	}

	types := *schema.Type

	if slices.Contains(types, "string") {
		return hclwrite.TokensForIdentifier("string"), nil
	}
	if slices.Contains(types, "integer") || slices.Contains(types, "number") {
		return hclwrite.TokensForIdentifier("number"), nil
	}
	if slices.Contains(types, "boolean") {
		return hclwrite.TokensForIdentifier("bool"), nil
	}
	if slices.Contains(types, "array") {
		elemType := hclwrite.TokensForIdentifier("any")
		if schema.Items != nil && schema.Items.Value != nil {
			var err error
			elemType, err = mapType(schema.Items.Value)
			if err != nil {
				return nil, err
			}
		}
		return hclwrite.TokensForFunctionCall("list", elemType), nil
	}
	if slices.Contains(types, "object") {
		// Get effective properties and required for allOf handling
		effectiveProps, err := openapi.GetEffectiveProperties(schema)
		if err != nil {
			return nil, fmt.Errorf("getting effective properties: %w", err)
		}
		effectiveRequired, err := openapi.GetEffectiveRequired(schema)
		if err != nil {
			return nil, fmt.Errorf("getting effective required: %w", err)
		}

		if len(effectiveProps) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				valueType, err := mapType(schema.AdditionalProperties.Schema.Value)
				if err != nil {
					return nil, err
				}
				return hclwrite.TokensForFunctionCall("map", valueType), nil
			}
			return hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")), nil
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
			fieldType, err := mapType(prop.Value)
			if err != nil {
				return nil, err
			}

			// Check if optional
			isOptional := true
			if slices.Contains(effectiveRequired, k) {
				isOptional = false
			}

			if isOptional {
				fieldType = hclwrite.TokensForFunctionCall("optional", fieldType)
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForIdentifier(naming.ToSnakeCase(k)),
				Value: fieldType,
			})
		}
		return hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject(attrs)), nil
	}

	return hclwrite.TokensForIdentifier("any"), nil
}

func buildNestedDescription(schema *openapi3.Schema, indent string) (string, error) {
	var sb strings.Builder

	// Get effective properties for allOf handling
	effectiveProps, err := openapi.GetEffectiveProperties(schema)
	if err != nil {
		return "", fmt.Errorf("getting effective properties in buildNestedDescription: %w", err)
	}

	type keyPair struct {
		original string
		snake    string
	}
	var childKeys []keyPair
	for k := range effectiveProps {
		childKeys = append(childKeys, keyPair{original: k, snake: naming.ToSnakeCase(k)})
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
			return "", fmt.Errorf("getting effective properties for nested object: %w", err)
		}
		if isNested && len(nestedProps) > 0 {
			nested, err := buildNestedDescription(val, indent+"  ")
			if err != nil {
				return "", err
			}
			sb.WriteString(nested)
		}
	}
	return sb.String(), nil
}
