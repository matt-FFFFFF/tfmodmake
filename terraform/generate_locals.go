package terraform

import (
	"sort"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/zclconf/go-cty/cty"
)

func buildLocals(rs *schema.ResourceSchema, localName string, supportsIdentity bool, secrets []secretField, resourceType string, caps InterfaceCapabilities, moduleNamePrefix string) (*hclwrite.File, error) {
	if rs == nil {
		return nil, nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	locals := body.AppendNewBlock("locals", nil)
	localBody := locals.Body()

	secretPaths := newSecretPathSet(secrets)

	// Build a synthetic root property from the ResourceSchema
	rootProp := &schema.Property{
		Type:     schema.TypeObject,
		Children: rs.Properties,
	}
	valueExpression, err := constructValue(rootProp, hclwrite.TokensForIdentifier("var"), true, secretPaths, "", supportsIdentity, moduleNamePrefix)
	if err != nil {
		return nil, err
	}
	localBody.SetAttributeRaw(localName, valueExpression)

	// Managed identity scaffolding (only when the resource schema supports configuring identity).
	if supportsIdentity {
		localBody.SetAttributeRaw("managed_identities", tokensForManagedIdentitiesLocal())
	}

	// Private endpoints local with opinionated defaults for subresource_name
	// Only generate when schema indicates private endpoint support
	if caps.SupportsPrivateEndpoints {
		localBody.SetAttributeRaw("private_endpoints", tokensForPrivateEndpointsLocal(resourceType))
	}

	return file, nil
}

func generateLocals(rs *schema.ResourceSchema, localName string, supportsIdentity bool, secrets []secretField, resourceType string, caps InterfaceCapabilities, moduleNamePrefix string, outputDir string) error {
	file, err := buildLocals(rs, localName, supportsIdentity, secrets, resourceType, caps, moduleNamePrefix)
	if err != nil {
		return err
	}
	if file == nil {
		return nil
	}
	return hclgen.WriteFileToDir(outputDir, "locals.tf", file)
}

func constructFlattenedRootPropertiesValue(prop *schema.Property, accessPath hclwrite.Tokens, secretPaths map[string]struct{}, moduleNamePrefix string) (hclwrite.Tokens, error) {
	// prop represents the schema property at root.properties.
	// The Terraform variables are flattened to var.<child> rather than var.properties.<child>.

	if prop == nil {
		return hclwrite.TokensForIdentifier("null"), nil
	}

	var attrs []hclwrite.ObjectAttrTokens
	var keys []string
	for k := range prop.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Keep object construction simple; AzAPI can ignore null properties when
	// ignore_null_property is enabled on the resource.
	for _, k := range keys {
		child := prop.Children[k]
		if child == nil {
			continue
		}
		if !isWritableProperty(child) {
			continue
		}

		if secretPaths != nil {
			if _, ok := secretPaths["properties."+k]; ok {
				continue
			}
		}

		snakeName := naming.ToSnakeCase(k)
		// Rename variables that conflict with Terraform module meta-arguments
		if moduleNamePrefix != "" && snakeName == "version" {
			snakeName = moduleNamePrefix + "_version"
		}
		var childAccess hclwrite.Tokens
		childAccess = append(childAccess, accessPath...)
		childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
		childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

		childValue, err := constructValue(child, childAccess, false, secretPaths, "properties."+k, false, moduleNamePrefix)
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, hclwrite.ObjectAttrTokens{
			Name:  tokensForObjectKey(k),
			Value: childValue,
		})
	}

	return hclwrite.TokensForObject(attrs), nil
}

func constructValue(prop *schema.Property, accessPath hclwrite.Tokens, isRoot bool, secretPaths map[string]struct{}, pathPrefix string, omitRootIdentity bool, moduleNamePrefix string) (hclwrite.Tokens, error) {
	if prop.Type == schema.TypeObject {
		if len(prop.Children) == 0 {
			if prop.AdditionalProperties != nil {
				mappedValue, err := constructValue(prop.AdditionalProperties, hclwrite.TokensForIdentifier("value"), false, secretPaths, pathPrefix, false, moduleNamePrefix)
				if err != nil {
					return nil, err
				}

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
					return hclgen.NullEqualityTernary(accessPath, tokens), nil
				}
				return tokens, nil
			}
			return accessPath, nil // map(string) or free-form, passed as is
		}

		var attrs []hclwrite.ObjectAttrTokens
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

			// Identity is configured via the azapi_resource identity block when supported.
			// Avoid including it in the request body locals.
			if isRoot && omitRootIdentity && k == "identity" {
				continue
			}

			// Location is configured via the azapi_resource location argument.
			// Avoid including it in the request body locals.
			if isRoot && k == "location" {
				continue
			}

			if !isWritableProperty(child) {
				continue
			}

			childPath := k
			if pathPrefix != "" {
				childPath = pathPrefix + "." + k
			}
			if secretPaths != nil {
				if _, ok := secretPaths[childPath]; ok {
					continue
				}
			}

			// Flatten the top-level "properties" bag into separate variables.
			if isRoot && k == "properties" && child.Type == schema.TypeObject && len(child.Children) > 0 {
				childValue, err := constructFlattenedRootPropertiesValue(child, accessPath, secretPaths, moduleNamePrefix)
				if err != nil {
					return nil, err
				}
				attrs = append(attrs, hclwrite.ObjectAttrTokens{
					Name:  tokensForObjectKey(k),
					Value: childValue,
				})
				continue
			}

			snakeName := naming.ToSnakeCase(k)
			var childAccess hclwrite.Tokens
			childAccess = append(childAccess, accessPath...)
			childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
			childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

			childValue, err := constructValue(child, childAccess, false, secretPaths, childPath, false, moduleNamePrefix)
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  tokensForObjectKey(k),
				Value: childValue,
			})
		}

		objTokens := hclwrite.TokensForObject(attrs)
		if !isRoot {
			return hclgen.NullEqualityTernary(accessPath, objTokens), nil
		}
		return objTokens, nil
	}

	if prop.Type == schema.TypeArray {
		if prop.ItemType != nil {
			childValue, err := constructValue(prop.ItemType, hclwrite.TokensForIdentifier("item"), false, secretPaths, pathPrefix+"[]", false, moduleNamePrefix)
			if err != nil {
				return nil, err
			}

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
				return hclgen.NullEqualityTernary(accessPath, tokens), nil
			}
			return tokens, nil
		}
		return accessPath, nil
	}

	return accessPath, nil
}

func tokensForManagedIdentitiesLocal() hclwrite.Tokens {
	varManaged := hclgen.TokensForTraversal("var", "managed_identities")
	userAssigned := append(hclwrite.Tokens(nil), varManaged...)
	userAssigned = append(userAssigned, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
	userAssigned = append(userAssigned, hclwrite.TokensForIdentifier("user_assigned_resource_ids")...)

	systemAssigned := append(hclwrite.Tokens(nil), varManaged...)
	systemAssigned = append(systemAssigned, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
	systemAssigned = append(systemAssigned, hclwrite.TokensForIdentifier("system_assigned")...)

	lengthUserAssigned := hclwrite.TokensForFunctionCall("length", userAssigned)
	userAssignedGtZero := append(hclwrite.Tokens(nil), lengthUserAssigned...)
	userAssignedGtZero = append(userAssignedGtZero, &hclwrite.Token{Type: hclsyntax.TokenGreaterThan, Bytes: []byte(">")})
	userAssignedGtZero = append(userAssignedGtZero, hclwrite.TokensForValue(cty.NumberIntVal(0))...)

	condAny := append(hclwrite.Tokens(nil), systemAssigned...)
	condAny = append(condAny, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte("||")})
	condAny = append(condAny, userAssignedGtZero...)

	bothCond := append(hclwrite.Tokens(nil), systemAssigned...)
	bothCond = append(bothCond, &hclwrite.Token{Type: hclsyntax.TokenAnd, Bytes: []byte("&&")})
	bothCond = append(bothCond, userAssignedGtZero...)

	typeExpr := append(hclwrite.Tokens(nil), bothCond...)
	typeExpr = append(typeExpr, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	typeExpr = append(typeExpr, hclwrite.TokensForValue(cty.StringVal("SystemAssigned, UserAssigned"))...)
	typeExpr = append(typeExpr, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	typeExpr = append(typeExpr, userAssignedGtZero...)
	typeExpr = append(typeExpr, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	typeExpr = append(typeExpr, hclwrite.TokensForValue(cty.StringVal("UserAssigned"))...)
	typeExpr = append(typeExpr, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	typeExpr = append(typeExpr, hclwrite.TokensForValue(cty.StringVal("SystemAssigned"))...)

	identityThisObject := hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
		{Name: hclwrite.TokensForIdentifier("type"), Value: typeExpr},
		{Name: hclwrite.TokensForIdentifier("user_assigned_resource_ids"), Value: userAssigned},
	})

	identityThis := hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
		{Name: hclwrite.TokensForIdentifier("this"), Value: identityThisObject},
	})

	emptyObj := hclwrite.TokensForObject(nil)

	systemAssignedUserAssigned := append(hclwrite.Tokens(nil), condAny...)
	systemAssignedUserAssigned = append(systemAssignedUserAssigned, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	systemAssignedUserAssigned = append(systemAssignedUserAssigned, identityThis...)
	systemAssignedUserAssigned = append(systemAssignedUserAssigned, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	systemAssignedUserAssigned = append(systemAssignedUserAssigned, emptyObj...)

	systemAssignedOnly := append(hclwrite.Tokens(nil), systemAssigned...)
	systemAssignedOnly = append(systemAssignedOnly, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	systemAssignedOnly = append(systemAssignedOnly, hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
		{Name: hclwrite.TokensForIdentifier("this"), Value: hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("type"), Value: hclwrite.TokensForValue(cty.StringVal("SystemAssigned"))},
		})},
	})...)
	systemAssignedOnly = append(systemAssignedOnly, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	systemAssignedOnly = append(systemAssignedOnly, emptyObj...)

	userAssignedOnly := append(hclwrite.Tokens(nil), userAssignedGtZero...)
	userAssignedOnly = append(userAssignedOnly, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	userAssignedOnly = append(userAssignedOnly, hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
		{Name: hclwrite.TokensForIdentifier("this"), Value: hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("type"), Value: hclwrite.TokensForValue(cty.StringVal("UserAssigned"))},
			{Name: hclwrite.TokensForIdentifier("user_assigned_resource_ids"), Value: userAssigned},
		})},
	})...)
	userAssignedOnly = append(userAssignedOnly, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	userAssignedOnly = append(userAssignedOnly, emptyObj...)

	return hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
		{Name: hclwrite.TokensForIdentifier("system_assigned_user_assigned"), Value: systemAssignedUserAssigned},
		{Name: hclwrite.TokensForIdentifier("system_assigned"), Value: systemAssignedOnly},
		{Name: hclwrite.TokensForIdentifier("user_assigned"), Value: userAssignedOnly},
	})
}

// tokensForPrivateEndpointsLocal generates the local.private_endpoints expression that provides
// opinionated defaults for subresource_name based on the resource type.
func tokensForPrivateEndpointsLocal(resourceType string) hclwrite.Tokens {
	// Build the for expression: {
	//   for k, v in var.private_endpoints : k => merge(
	//     v,
	//     {
	//       subresource_name = coalesce(v.subresource_name, "<default>")
	//     }
	//   )
	// }
	//
	// If there's no default mapping for this resource type, use:
	// {
	//   for k, v in var.private_endpoints : k => v
	// }

	// Look up a default subresource name for this resource type.
	// Only default when the docs list exactly one possible subresource.
	defaultSubresource, hasDefault := privateEndpointDefaultSubresource(resourceType)

	varPE := hclgen.TokensForTraversal("var", "private_endpoints")

	valueTokens := hclwrite.TokensForIdentifier("v")

	if hasDefault {
		vSubresource := hclgen.TokensForTraversal("v", "subresource_name")
		coalesceCall := hclwrite.TokensForFunctionCall("coalesce", vSubresource, hclwrite.TokensForValue(cty.StringVal(defaultSubresource)))

		mergeArg := hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("subresource_name"), Value: coalesceCall},
		})
		valueTokens = hclwrite.TokensForFunctionCall("merge", hclwrite.TokensForIdentifier("v"), mergeArg)
	}

	var tokens hclwrite.Tokens
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("for")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("k")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("v")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in")})
	tokens = append(tokens, varPE...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("k")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenFatArrow, Bytes: []byte("=>")})
	tokens = append(tokens, valueTokens...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})

	return tokens
}
