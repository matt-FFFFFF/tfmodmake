package terraform

import (
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
)

// secretField represents a secret field detected in the schema.
type secretField struct {
	// path is the JSON path to the field, e.g., "properties.daprAIInstrumentationKey"
	path string
	// varName is the snake_case variable name, e.g., "dapr_ai_instrumentation_key"
	varName string
	// prop is the schema property for this field
	prop *schema.Property
}

// isSecretField checks if a property should be treated as a secret.
func isSecretField(prop *schema.Property) bool {
	if prop == nil {
		return false
	}
	return prop.Sensitive || prop.WriteOnly
}

// isArrayProperty checks if a property is an array type.
func isArrayProperty(prop *schema.Property) bool {
	if prop == nil {
		return false
	}
	return prop.Type == schema.TypeArray
}

// schemaContainsSecretFields checks if a property or any of its children contain secrets.
func schemaContainsSecretFields(prop *schema.Property) bool {
	if prop == nil {
		return false
	}

	if isSecretField(prop) {
		return true
	}

	if isArrayProperty(prop) {
		if prop.ItemType == nil {
			return false
		}
		return schemaContainsSecretFields(prop.ItemType)
	}

	if prop.Type != schema.TypeObject {
		return false
	}

	for _, child := range prop.Children {
		if child == nil {
			continue
		}
		if !isWritableProperty(child) {
			continue
		}
		if schemaContainsSecretFields(child) {
			return true
		}
	}

	if prop.AdditionalProperties != nil {
		if schemaContainsSecretFields(prop.AdditionalProperties) {
			return true
		}
	}

	return false
}

// collectSecretFields walks the ResourceSchema and collects all secret fields.
func collectSecretFields(rs *schema.ResourceSchema) []secretField {
	if rs == nil {
		return nil
	}

	var secrets []secretField

	var keys []string
	for k := range rs.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		prop := rs.Properties[name]
		if prop == nil {
			continue
		}
		if !isWritableProperty(prop) {
			continue
		}
		secrets = collectSecretFieldsRecursive(prop, name, secrets)
	}

	return secrets
}

// collectSecretFieldsRecursive traverses property trees to find secret fields.
func collectSecretFieldsRecursive(prop *schema.Property, currentPath string, secrets []secretField) []secretField {
	if prop == nil {
		return secrets
	}

	if isSecretField(prop) {
		secrets = append(secrets, secretField{
			path:    currentPath,
			varName: naming.ToSnakeCase(lastPathSegment(currentPath)),
			prop:    prop,
		})
		return secrets
	}

	// Coarse-grained handling for secrets inside arrays of items:
	// if any of the item's fields are secret, treat the entire array property
	// as a single secret-bearing field at the array property path.
	if isArrayProperty(prop) && prop.ItemType != nil {
		if schemaContainsSecretFields(prop.ItemType) {
			secrets = append(secrets, secretField{
				path:    currentPath,
				varName: naming.ToSnakeCase(lastPathSegment(currentPath)),
				prop:    prop,
			})
			return secrets
		}
	}

	// Recursively check nested objects
	if prop.Type == schema.TypeObject && len(prop.Children) > 0 {
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
			childPath := currentPath + "." + childName
			secrets = collectSecretFieldsRecursive(child, childPath, secrets)
		}
	}

	return secrets
}

// lastPathSegment returns the last segment of a dot-separated path.
func lastPathSegment(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
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

// nullCheckFunc returns HCL tokens for an expression that should be null-checked
// when rendering a sensitive_body intermediate container at the given path segments.
// If the container should not be null-checked, it returns nil.
type nullCheckFunc func(pathSegments []string) hclwrite.Tokens

// tokensForSensitiveBody builds a nested HCL object expression for the sensitive_body
// attribute, reconstructing the path hierarchy from flat secret field paths.
//
// The nullCheckFor callback allows intermediate container objects to be wrapped with
// null-equality ternaries so that partial objects aren't emitted when the parent
// variable is null. This prevents AzAPI schema validation errors where a container
// has required non-secret siblings that are absent from sensitive_body.
func tokensForSensitiveBody(secrets []secretField, valueFor func(secretField) hclwrite.Tokens, nullCheckFor nullCheckFunc) hclwrite.Tokens {
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

	var render func(node *sensitiveBodyNode, pathSegments []string) hclwrite.Tokens
	render = func(node *sensitiveBodyNode, pathSegments []string) hclwrite.Tokens {
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
			childPath := append(pathSegments, k) //nolint:gocritic // intentionally creating new slice per child
			var value hclwrite.Tokens
			if child != nil && child.secret != nil && len(child.children) == 0 {
				value = valueFor(*child.secret)
			} else {
				childObj := render(child, childPath)
				if nullCheckFor != nil {
					if checkExpr := nullCheckFor(childPath); checkExpr != nil {
						value = hclgen.NullEqualityTernary(checkExpr, childObj)
					} else {
						value = childObj
					}
				} else {
					value = childObj
				}
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  tokensForObjectKey(k),
				Value: value,
			})
		}
		return hclwrite.TokensForObject(attrs)
	}

	return render(root, nil)
}
