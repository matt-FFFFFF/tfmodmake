package terraform

import (
	"strings"

	"github.com/matt-FFFFFF/tfmodmake/schema"
)

// isWritableProperty checks if a property is writable (not read-only).
func isWritableProperty(prop *schema.Property) bool {
	if prop == nil {
		return false
	}
	return !prop.ReadOnly
}

// hasWritableProperty checks if a named property at the given dot-path is writable.
func hasWritableProperty(rs *schema.ResourceSchema, path string) bool {
	if rs == nil || path == "" {
		return false
	}
	prop := navigateToProperty(rs.Properties, path)
	return prop != nil && !prop.ReadOnly
}

// navigateToProperty follows a dot-separated path through nested properties.
func navigateToProperty(props map[string]*schema.Property, path string) *schema.Property {
	segments := strings.Split(path, ".")
	current := props
	var prop *schema.Property
	for _, seg := range segments {
		if current == nil {
			return nil
		}
		p, ok := current[seg]
		if !ok {
			return nil
		}
		prop = p
		current = p.Children
	}
	return prop
}
