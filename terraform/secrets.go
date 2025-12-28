package terraform

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
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

// isSecretField checks if a schema property should be treated as a secret by
// checking writeOnly, x-ms-secret extension, or description-based heuristics.
func isSecretField(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}

	// OpenAPI 3 writeOnly is a strong signal that the field is sensitive.
	if schema.WriteOnly {
		return true
	}

	// Some Azure specs don't consistently mark secrets with x-ms-secret, but do
	// document that a value is never returned. Treat those as secrets to avoid
	// leaking them into `body`.
	if schema.Description != "" {
		desc := strings.ToLower(schema.Description)
		if strings.Contains(desc, "never be returned") {
			return true
		}
	}

	if schema.Extensions == nil {
		return false
	}
	if val, ok := schema.Extensions["x-ms-secret"]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false
}

func isArraySchema(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.Type != nil && slices.Contains(*schema.Type, "array") {
		return true
	}
	return schema.Items != nil
}

func schemaContainsSecretFields(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}

	if isSecretField(schema) {
		return true
	}

	if isArraySchema(schema) {
		if schema.Items == nil || schema.Items.Value == nil {
			return false
		}
		return schemaContainsSecretFields(schema.Items.Value)
	}

	if schema.Type == nil || !slices.Contains(*schema.Type, "object") {
		return false
	}

	props, err := openapi.GetEffectiveProperties(schema)
	if err != nil {
		panic(fmt.Sprintf("failed to get effective properties while scanning for secret fields: %v", err))
	}

	for _, prop := range props {
		if prop == nil || prop.Value == nil {
			continue
		}
		if !isWritableProperty(prop.Value) {
			continue
		}
		if schemaContainsSecretFields(prop.Value) {
			return true
		}
	}

	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		if schemaContainsSecretFields(schema.AdditionalProperties.Schema.Value) {
			return true
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
		if prop == nil || prop.Value == nil {
			continue
		}
		if !isWritableProperty(prop.Value) {
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

		// Coarse-grained handling for secrets inside arrays of items:
		// if any of the item schema's fields are secret, treat the entire array property
		// as a single secret-bearing field at the array property path (no [] segments).
		//
		// This enables:
		//   - moving the whole array off the regular body into sensitive_body,
		//   - generating a single <var>_version for lifecycle forcing, and
		//   - avoiding invalid HCL keys like "secrets[]".
		if !isSecretField(propSchema) && isArraySchema(propSchema) {
			if propSchema.Items != nil && propSchema.Items.Value != nil && schemaContainsSecretFields(propSchema.Items.Value) {
				secrets = append(secrets, secretField{
					path:    currentPath,
					varName: toSnakeCase(name),
					schema:  propSchema,
				})
				continue
			}
		}

		// Recursively check nested objects
		if propSchema.Type != nil && slices.Contains(*propSchema.Type, "object") && len(propSchema.Properties) > 0 {
			nested := collectSecretFields(propSchema, currentPath)
			secrets = append(secrets, nested...)
		}

		// NOTE: For arrays whose item schema contains secret fields, we intentionally treat the
		// whole array as a single secret-bearing field (see coarse-grained logic above).
		// This avoids invalid keys like "secrets[]" while still keeping secrets out of `body`.
	}

	return secrets
}

func newSecretPathSet(secrets []secretField) map[string]struct{} {
	if len(secrets) == 0 {
		return nil
	}
	paths := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		p := strings.TrimSpace(secret.path)
		if p == "" {
			continue
		}
		paths[p] = struct{}{}
	}
	return paths
}

type sensitiveBodyNode struct {
	children map[string]*sensitiveBodyNode
	secret   *secretField
}

func (n *sensitiveBodyNode) ensureChild(key string) *sensitiveBodyNode {
	if n.children == nil {
		n.children = make(map[string]*sensitiveBodyNode)
	}
	child, ok := n.children[key]
	if !ok {
		child = &sensitiveBodyNode{}
		n.children[key] = child
	}
	return child
}

func tokensForSensitiveBody(secrets []secretField, valueFor func(secretField) hclwrite.Tokens) hclwrite.Tokens {
	root := &sensitiveBodyNode{}
	for i := range secrets {
		path := strings.TrimSpace(secrets[i].path)
		if path == "" {
			continue
		}
		segments := strings.Split(path, ".")
		node := root
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			node = node.ensureChild(seg)
		}
		node.secret = &secrets[i]
	}

	var render func(node *sensitiveBodyNode) hclwrite.Tokens
	render = func(node *sensitiveBodyNode) hclwrite.Tokens {
		if node == nil || len(node.children) == 0 {
			return hclwrite.TokensForObject(nil)
		}
		keys := make([]string, 0, len(node.children))
		for k := range node.children {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		attrs := make([]hclwrite.ObjectAttrTokens, 0, len(keys))
		for _, k := range keys {
			child := node.children[k]
			var value hclwrite.Tokens
			if child != nil && child.secret != nil && len(child.children) == 0 {
				value = valueFor(*child.secret)
			} else {
				value = render(child)
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  tokensForObjectKey(k),
				Value: value,
			})
		}
		return hclwrite.TokensForObject(attrs)
	}

	return render(root)
}
