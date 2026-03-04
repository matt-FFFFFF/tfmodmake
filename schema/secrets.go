package schema

// SecretField represents a property path that contains sensitive data.
type SecretField struct {
	// Path is the dot-separated path to the secret property (e.g. "properties.adminPassword").
	Path string

	// Property is the schema property that is sensitive.
	Property *Property
}

// IsSensitive checks if a property is marked as sensitive.
func IsSensitive(prop *Property) bool {
	return prop != nil && prop.Sensitive
}

// CollectSecretFields walks the property tree and collects all sensitive fields.
func CollectSecretFields(schema *ResourceSchema) []SecretField {
	if schema == nil {
		return nil
	}

	var secrets []SecretField
	for name, prop := range schema.Properties {
		secrets = collectSecretsRecursive(prop, name, secrets)
	}
	return secrets
}

// collectSecretsRecursive recursively walks properties to find sensitive fields.
func collectSecretsRecursive(prop *Property, path string, secrets []SecretField) []SecretField {
	if prop == nil {
		return secrets
	}

	if prop.Sensitive {
		secrets = append(secrets, SecretField{
			Path:     path,
			Property: prop,
		})
	}

	// Recurse into children
	for name, child := range prop.Children {
		childPath := path + "." + name
		secrets = collectSecretsRecursive(child, childPath, secrets)
	}

	// Recurse into array item type
	if prop.ItemType != nil {
		secrets = collectSecretsRecursive(prop.ItemType, path+"[]", secrets)
	}

	return secrets
}
