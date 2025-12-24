package terraform

import (
	"fmt"
	"slices"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
)

func generateLocals(schema *openapi3.Schema, localName string, secrets []secretField) error {
	if schema == nil {
		return nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	locals := body.AppendNewBlock("locals", nil)
	localBody := locals.Body()

	secretPaths := newSecretPathSet(secrets)
	valueExpression := constructValue(schema, hclwrite.TokensForIdentifier("var"), true, secretPaths, "")
	localBody.SetAttributeRaw(localName, valueExpression)

	return hclgen.WriteFile("locals.tf", file)
}

func constructFlattenedRootPropertiesValue(schema *openapi3.Schema, accessPath hclwrite.Tokens, secretPaths map[string]struct{}) hclwrite.Tokens {
	// schema represents the OpenAPI schema at root.properties.
	// The Terraform variables are flattened to var.<child> rather than var.properties.<child>.

	if schema == nil {
		return hclwrite.TokensForIdentifier("null")
	}

	// Get effective properties for allOf handling
	effectiveProps, err := openapi.GetEffectiveProperties(schema)
	if err != nil {
		// Errors indicate cycles or conflicts which should fail generation
		panic(fmt.Sprintf("failed to get effective properties in constructFlattenedRootPropertiesValue: %v", err))
	}

	var attrs []hclwrite.ObjectAttrTokens
	var keys []string
	for k := range effectiveProps {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Keep object construction simple; AzAPI can ignore null properties when
	// ignore_null_property is enabled on the resource.
	for _, k := range keys {
		prop := effectiveProps[k]
		if prop == nil || prop.Value == nil {
			continue
		}
		if !isWritableProperty(prop.Value) {
			continue
		}

		if secretPaths != nil {
			if _, ok := secretPaths["properties."+k]; ok {
				continue
			}
		}

		snakeName := toSnakeCase(k)
		var childAccess hclwrite.Tokens
		childAccess = append(childAccess, accessPath...)
		childAccess = append(childAccess, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
		childAccess = append(childAccess, hclwrite.TokensForIdentifier(snakeName)...)

		childValue := constructValue(prop.Value, childAccess, false, secretPaths, "properties."+k)
		attrs = append(attrs, hclwrite.ObjectAttrTokens{
			Name:  tokensForObjectKey(k),
			Value: childValue,
		})
	}

	return hclwrite.TokensForObject(attrs)
}

func constructValue(schema *openapi3.Schema, accessPath hclwrite.Tokens, isRoot bool, secretPaths map[string]struct{}, pathPrefix string) hclwrite.Tokens {
	if schema.Type == nil {
		return accessPath
	}

	types := *schema.Type

	if slices.Contains(types, "object") {
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
				mappedValue := constructValue(schema.AdditionalProperties.Schema.Value, hclwrite.TokensForIdentifier("value"), false, secretPaths, pathPrefix)

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

		// Get effective properties for allOf handling
		effectiveProps, err := openapi.GetEffectiveProperties(schema)
		if err != nil {
			// Errors indicate cycles or conflicts which should fail generation
			panic(fmt.Sprintf("failed to get effective properties in constructValue: %v", err))
		}

		var attrs []hclwrite.ObjectAttrTokens
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
			if isRoot && k == "properties" && prop.Value.Type != nil && slices.Contains(*prop.Value.Type, "object") && len(prop.Value.Properties) > 0 {
				childValue := constructFlattenedRootPropertiesValue(prop.Value, accessPath, secretPaths)
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

			childValue := constructValue(prop.Value, childAccess, false, secretPaths, childPath)
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
			childValue := constructValue(schema.Items.Value, hclwrite.TokensForIdentifier("item"), false, secretPaths, pathPrefix+"[]")

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
