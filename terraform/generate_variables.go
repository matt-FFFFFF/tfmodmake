package terraform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/zclconf/go-cty/cty"
)

func buildVariables(rs *schema.ResourceSchema, supportsTags, supportsLocation, supportsIdentity bool, secrets []secretField, caps InterfaceCapabilities, moduleNamePrefix string) (*hclwrite.File, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	arrayItemsContainSecret := func(prop *schema.Property) bool {
		if prop == nil {
			return false
		}
		if prop.Type != schema.TypeArray {
			return false
		}
		if prop.ItemType == nil {
			return false
		}
		itemProp := prop.ItemType
		if itemProp.Type != schema.TypeObject {
			return false
		}
		for _, child := range itemProp.Children {
			if child == nil {
				continue
			}
			if !isWritableProperty(child) {
				continue
			}
			if isSecretField(child) {
				return true
			}
		}
		return false
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

	appendSchemaVariable := func(tfName, originalName string, prop *schema.Property) (*hclwrite.Body, error) {
		if prop == nil {
			return nil, nil
		}

		tfType, err := mapType(prop)
		if err != nil {
			return nil, err
		}

		var nestedDocProp *schema.Property
		if prop.Type == schema.TypeObject {
			switch {
			case len(prop.Children) > 0:
				nestedDocProp = prop
			case prop.AdditionalProperties != nil:
				apProp := prop.AdditionalProperties
				if apProp.Type == schema.TypeObject && len(apProp.Children) > 0 {
					nestedDocProp = apProp
				}
			}
		}
		isNestedObject := nestedDocProp != nil

		varBody := appendVariable(tfName, "", tfType)

		if isNestedObject {
			var sb strings.Builder
			desc := prop.Description
			if desc == "" {
				if originalName != "" {
					desc = fmt.Sprintf("The %s of the resource.", originalName)
				} else {
					desc = fmt.Sprintf("The %s of the resource.", tfName)
				}
			}
			sb.WriteString(desc)
			sb.WriteString("\n\n")

			if nestedDocProp != prop {
				sb.WriteString("Map values:\n")
			}

			nested := buildNestedDescription(nestedDocProp, "")
			sb.WriteString(nested)
			hclgen.SetDescriptionAttribute(varBody, sb.String())
		} else {
			description := prop.Description
			if description == "" {
				if originalName != "" {
					description = fmt.Sprintf("The %s of the resource.", originalName)
				} else {
					description = fmt.Sprintf("The %s of the resource.", tfName)
				}
			}
			hclgen.SetDescriptionAttribute(varBody, description)
		}

		if !prop.Required {
			varBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
		}

		// Mark secret fields as ephemeral
		if _, ok := secretVarNames[tfName]; ok {
			varBody.SetAttributeValue("ephemeral", cty.True)
		}

		// If this is an array of objects that contains secret fields in its items,
		// mark the whole variable as ephemeral. We currently don't generate array-aware
		// sensitive_body, so this prevents secrets from persisting in state.
		if arrayItemsContainSecret(prop) {
			varBody.SetAttributeValue("ephemeral", cty.True)
		}

		// Generate validations for this variable
		generateValidations(varBody, tfName, prop, prop.Required)
		if prop.Type == schema.TypeObject && len(prop.Children) > 0 {
			if err := generateNestedObjectValidations(varBody, tfName, prop); err != nil {
				return nil, err
			}
		}

		return varBody, nil
	}

	appendVariable("name", "The name of the resource.", hclwrite.TokensForIdentifier("string"))
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

	// Get top-level properties from the resource schema
	var keys []string
	if rs != nil {
		for k := range rs.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	for i, name := range keys {
		prop := rs.Properties[name]
		if prop == nil {
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

		// Flatten the standard ARM top-level "properties" bag into individual Terraform variables.
		// This is the default module shape for full-schema generation (no -root), per DESIGN.md.
		if name == "properties" && prop.Type == schema.TypeObject {
			if len(prop.Children) == 0 {
				continue
			}

			var childKeys []string
			for k := range prop.Children {
				childKeys = append(childKeys, k)
			}
			sort.Strings(childKeys)

			for _, childName := range childKeys {
				child := prop.Children[childName]
				if child == nil {
					continue
				}
				if !isWritableProperty(child) {
					continue
				}

				tfName := naming.ToSnakeCase(childName)
				if tfName == "" {
					return nil, fmt.Errorf("could not derive terraform variable name for %s", childName)
				}
				// Rename variables that conflict with Terraform module meta-arguments
				if moduleNamePrefix != "" && tfName == "version" {
					tfName = moduleNamePrefix + "_version"
				}

				// A collision under flattened root properties is a hard error: users would have no way
				// to configure that field.
				if _, reserved := reservedNames[tfName]; reserved {
					return nil, fmt.Errorf("terraform variable name collision: %q (from properties.%s)", tfName, childName)
				}
				if _, exists := seenNames[tfName]; exists {
					return nil, fmt.Errorf("terraform variable name collision: %q (from properties.%s)", tfName, childName)
				}
				seenNames[tfName] = struct{}{}

				if _, err := appendSchemaVariable(tfName, childName, child); err != nil {
					return nil, err
				}

				body.AppendNewline()
			}

			continue
		}

		if !isWritableProperty(prop) {
			continue
		}

		tfName := naming.ToSnakeCase(name)
		if tfName == "" {
			return nil, fmt.Errorf("could not derive terraform variable name for %s", name)
		}
		if _, reserved := reservedNames[tfName]; reserved {
			continue
		}
		// Rename variables that conflict with Terraform module meta-arguments
		if moduleNamePrefix != "" && tfName == "version" {
			tfName = moduleNamePrefix + "_version"
		}
		if _, exists := seenNames[tfName]; exists {
			return nil, fmt.Errorf("terraform variable name collision: %q (from %s)", tfName, name)
		}
		seenNames[tfName] = struct{}{}
		if _, err := appendSchemaVariable(tfName, name, prop); err != nil {
			return nil, err
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

		tfType, err := mapType(secret.prop)
		if err != nil {
			return nil, err
		}
		description := ""
		if secret.prop != nil {
			description = secret.prop.Description
		}
		secretVarBody := appendVariable(
			secret.varName,
			description,
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
			return nil, fmt.Errorf("terraform variable name collision: %q (from secret version var)", versionVarName)
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

	return file, nil
}

func generateVariables(rs *schema.ResourceSchema, supportsTags, supportsLocation, supportsIdentity bool, secrets []secretField, caps InterfaceCapabilities, moduleNamePrefix string, outputDir string) error {
	file, err := buildVariables(rs, supportsTags, supportsLocation, supportsIdentity, secrets, caps, moduleNamePrefix)
	if err != nil {
		return err
	}
	return hclgen.WriteFileToDir(outputDir, "variables.tf", file)
}

func mapType(prop *schema.Property) (hclwrite.Tokens, error) {
	if prop == nil {
		return hclwrite.TokensForIdentifier("any"), nil
	}

	switch prop.Type {
	case schema.TypeString:
		return hclwrite.TokensForIdentifier("string"), nil
	case schema.TypeInteger:
		return hclwrite.TokensForIdentifier("number"), nil
	case schema.TypeBoolean:
		return hclwrite.TokensForIdentifier("bool"), nil
	case schema.TypeArray:
		elemType := hclwrite.TokensForIdentifier("any")
		if prop.ItemType != nil {
			var err error
			elemType, err = mapType(prop.ItemType)
			if err != nil {
				return nil, err
			}
		}
		return hclwrite.TokensForFunctionCall("list", elemType), nil
	case schema.TypeObject:
		if len(prop.Children) == 0 {
			if prop.AdditionalProperties != nil {
				valueType, err := mapType(prop.AdditionalProperties)
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
		for k := range prop.Children {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			child := prop.Children[k]
			if child == nil {
				continue
			}
			if !isWritableProperty(child) {
				continue
			}
			fieldType, err := mapType(child)
			if err != nil {
				return nil, err
			}

			// Check if optional
			if !child.Required {
				fieldType = hclwrite.TokensForFunctionCall("optional", fieldType)
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForIdentifier(naming.ToSnakeCase(k)),
				Value: fieldType,
			})
		}
		return hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject(attrs)), nil
	default:
		return hclwrite.TokensForIdentifier("any"), nil
	}
}

func buildNestedDescription(prop *schema.Property, indent string) string {
	var sb strings.Builder

	if prop == nil || len(prop.Children) == 0 {
		return ""
	}

	type keyPair struct {
		original string
		snake    string
	}
	var childKeys []keyPair
	for k := range prop.Children {
		childKeys = append(childKeys, keyPair{original: k, snake: naming.ToSnakeCase(k)})
	}
	sort.Slice(childKeys, func(i, j int) bool {
		return childKeys[i].snake < childKeys[j].snake
	})

	for _, pair := range childKeys {
		k := pair.original
		child := prop.Children[k]
		if child == nil {
			continue
		}

		if !isWritableProperty(child) {
			continue
		}

		childDesc := child.Description
		if childDesc == "" {
			childDesc = fmt.Sprintf("The %s property.", k)
		}
		childDesc = strings.ReplaceAll(childDesc, "\n", " ")

		sb.WriteString(fmt.Sprintf("%s- `%s` - %s\n", indent, pair.snake, childDesc))

		// Check if nested object has children
		if child.Type == schema.TypeObject && len(child.Children) > 0 {
			nested := buildNestedDescription(child, indent+"  ")
			sb.WriteString(nested)
		}
	}
	return sb.String()
}
