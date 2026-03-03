package schema

import (
	"fmt"

	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
)

// ConvertResource converts a loaded bicep-types resource into the internal ResourceSchema.
func ConvertResource(loaded *bicepdata.LoadedResource) (*ResourceSchema, error) {
	if loaded.ResourceType.Body == nil {
		return nil, fmt.Errorf("resource %s has no body type reference", loaded.ResourceTypeName)
	}

	bodyType, err := loaded.ResolveType(loaded.ResourceType.Body)
	if err != nil {
		return nil, fmt.Errorf("resolving body type for %s: %w", loaded.ResourceTypeName, err)
	}

	c := &converter{
		loaded:  loaded,
		visited: make(map[int]bool),
	}

	properties, err := c.convertBodyType(bodyType)
	if err != nil {
		return nil, fmt.Errorf("converting body type for %s: %w", loaded.ResourceTypeName, err)
	}

	rs := &ResourceSchema{
		Properties:   properties,
		ResourceType: loaded.ResourceTypeName,
		APIVersion:   loaded.APIVersion,
	}

	// Detect capabilities
	rs.SupportsTags = detectSupportsTags(rs)
	rs.SupportsLocation = detectSupportsLocation(rs)
	rs.SupportsIdentity = detectSupportsIdentity(rs)

	return rs, nil
}

// converter holds state during the recursive conversion of bicep types to schema properties.
type converter struct {
	loaded  *bicepdata.LoadedResource
	visited map[int]bool // cycle detection: type index -> visited
}

// convertBodyType converts the resource body (ObjectType or DiscriminatedObjectType) into a property map.
func (c *converter) convertBodyType(bodyType types.Type) (map[string]*Property, error) {
	switch bt := bodyType.(type) {
	case *types.ObjectType:
		return c.convertObjectProperties(bt.Properties)
	case *types.DiscriminatedObjectType:
		return c.convertDiscriminatedObject(bt)
	default:
		return nil, fmt.Errorf("unexpected body type %T (expected ObjectType or DiscriminatedObjectType)", bodyType)
	}
}

// convertObjectProperties converts a map of ObjectTypeProperty into schema Properties.
func (c *converter) convertObjectProperties(props map[string]types.ObjectTypeProperty) (map[string]*Property, error) {
	result := make(map[string]*Property, len(props))
	for name, objProp := range props {
		prop, err := c.convertObjectProperty(name, objProp)
		if err != nil {
			return nil, fmt.Errorf("converting property %q: %w", name, err)
		}
		result[name] = prop
	}
	return result, nil
}

// convertObjectProperty converts a single ObjectTypeProperty to a schema Property.
func (c *converter) convertObjectProperty(name string, objProp types.ObjectTypeProperty) (*Property, error) {
	prop := &Property{
		Name:               name,
		Description:        objProp.Description,
		Required:           IsRequired(objProp.Flags),
		ReadOnly:           IsReadOnly(objProp.Flags),
		WriteOnly:          IsWriteOnly(objProp.Flags),
		DeployTimeConstant: IsDeployTimeConstant(objProp.Flags),
	}

	if objProp.Type == nil {
		prop.Type = TypeAny
		return prop, nil
	}

	err := c.resolvePropertyType(prop, objProp.Type)
	if err != nil {
		return nil, err
	}

	return prop, nil
}

// resolvePropertyType resolves a type reference and populates the property's type info.
func (c *converter) resolvePropertyType(prop *Property, ref types.ITypeReference) error {
	resolved, err := c.loaded.ResolveType(ref)
	if err != nil {
		// If we can't resolve, treat as any
		prop.Type = TypeAny
		return nil
	}

	return c.applyResolvedType(prop, resolved, ref)
}

// applyResolvedType populates a property based on a resolved type.
func (c *converter) applyResolvedType(prop *Property, resolved types.Type, ref types.ITypeReference) error {
	switch t := resolved.(type) {
	case *types.StringType:
		prop.Type = TypeString
		prop.Sensitive = t.Sensitive
		if t.MinLength != nil {
			prop.Constraints.MinLength = t.MinLength
		}
		if t.MaxLength != nil {
			prop.Constraints.MaxLength = t.MaxLength
		}
		if t.Pattern != "" {
			prop.Constraints.Pattern = t.Pattern
		}

	case *types.IntegerType:
		prop.Type = TypeInteger
		if t.MinValue != nil {
			prop.Constraints.MinValue = t.MinValue
		}
		if t.MaxValue != nil {
			prop.Constraints.MaxValue = t.MaxValue
		}

	case *types.BooleanType:
		prop.Type = TypeBoolean

	case *types.ArrayType:
		prop.Type = TypeArray
		if t.MinLength != nil {
			prop.Constraints.MinItems = t.MinLength
		}
		if t.MaxLength != nil {
			prop.Constraints.MaxItems = t.MaxLength
		}
		if t.ItemType != nil {
			itemProp := &Property{Name: "item"}
			if err := c.resolvePropertyType(itemProp, t.ItemType); err != nil {
				return fmt.Errorf("resolving array item type: %w", err)
			}
			prop.ItemType = itemProp
		}

	case *types.ObjectType:
		prop.Type = TypeObject
		// Cycle detection
		idx := typeRefIndex(ref)
		if idx >= 0 {
			if c.visited[idx] {
				// Cycle detected, leave children empty
				return nil
			}
			c.visited[idx] = true
			defer delete(c.visited, idx)
		}
		children, err := c.convertObjectProperties(t.Properties)
		if err != nil {
			return fmt.Errorf("converting nested object properties: %w", err)
		}
		prop.Children = children

		// Handle additional properties
		if t.AdditionalProperties != nil {
			addProp := &Property{Name: "additional_properties"}
			if err := c.resolvePropertyType(addProp, t.AdditionalProperties); err != nil {
				return fmt.Errorf("resolving additional properties type: %w", err)
			}
			prop.AdditionalProperties = addProp
		}

		// Check object-level sensitivity
		if t.Sensitive != nil && *t.Sensitive {
			prop.Sensitive = true
		}

	case *types.DiscriminatedObjectType:
		prop.Type = TypeObject
		prop.Discriminator = t.Discriminator
		// Cycle detection
		idx := typeRefIndex(ref)
		if idx >= 0 {
			if c.visited[idx] {
				return nil
			}
			c.visited[idx] = true
			defer delete(c.visited, idx)
		}
		children, err := c.convertDiscriminatedObject(t)
		if err != nil {
			return fmt.Errorf("converting discriminated object: %w", err)
		}
		prop.Children = children

	case *types.UnionType:
		// Check if it's a string enum (union of StringLiteralType)
		enumValues, isStringEnum := c.extractStringEnum(t)
		if isStringEnum {
			prop.Type = TypeString
			prop.Enum = enumValues
		} else {
			// Non-string union, treat as any
			prop.Type = TypeAny
		}

	case *types.StringLiteralType:
		prop.Type = TypeString
		prop.Enum = []string{t.Value}
		prop.Sensitive = t.Sensitive

	case *types.NullType:
		prop.Type = TypeNull

	case *types.AnyType:
		prop.Type = TypeAny

	case *types.BuiltInType:
		prop.Type = TypeAny

	case *types.ResourceType:
		// Resource type reference - treat as object
		prop.Type = TypeObject

	default:
		prop.Type = TypeAny
	}

	return nil
}

// convertDiscriminatedObject flattens a DiscriminatedObjectType into a property map.
// It merges base properties with all variant properties, and adds the discriminator as a required enum.
func (c *converter) convertDiscriminatedObject(dot *types.DiscriminatedObjectType) (map[string]*Property, error) {
	result := make(map[string]*Property)

	// Convert base properties
	for name, objProp := range dot.BaseProperties {
		prop, err := c.convertObjectProperty(name, objProp)
		if err != nil {
			return nil, fmt.Errorf("converting base property %q: %w", name, err)
		}
		result[name] = prop
	}

	// Add discriminator as a required string enum property
	var discriminatorValues []string
	for value := range dot.Elements {
		discriminatorValues = append(discriminatorValues, value)
	}
	result[dot.Discriminator] = &Property{
		Name:     dot.Discriminator,
		Type:     TypeString,
		Required: true,
		Enum:     discriminatorValues,
	}

	// Merge properties from all variants
	for _, elementRef := range dot.Elements {
		resolved, err := c.loaded.ResolveType(elementRef)
		if err != nil {
			continue // Skip unresolvable variants
		}

		switch variant := resolved.(type) {
		case *types.ObjectType:
			for name, objProp := range variant.Properties {
				if _, exists := result[name]; exists {
					continue // Don't override existing properties (base or discriminator)
				}
				prop, err := c.convertObjectProperty(name, objProp)
				if err != nil {
					continue // Skip problematic properties
				}
				// Variant properties are not required (they're conditional on the discriminator value)
				prop.Required = false
				result[name] = prop
			}
		}
	}

	return result, nil
}

// extractStringEnum checks if a UnionType is a string enum (all elements are StringLiteralType)
// and returns the enum values if so.
func (c *converter) extractStringEnum(ut *types.UnionType) ([]string, bool) {
	var values []string
	for _, elemRef := range ut.Elements {
		resolved, err := c.loaded.ResolveType(elemRef)
		if err != nil {
			return nil, false
		}
		slt, ok := resolved.(*types.StringLiteralType)
		if !ok {
			return nil, false
		}
		values = append(values, slt.Value)
	}
	return values, len(values) > 0
}

// typeRefIndex extracts the integer index from a type reference, or -1 if not applicable.
func typeRefIndex(ref types.ITypeReference) int {
	switch r := ref.(type) {
	case *types.TypeReference:
		return r.Ref
	case types.TypeReference:
		return r.Ref
	default:
		return -1
	}
}

// detectSupportsTags checks if the resource has a writable "tags" property.
func detectSupportsTags(rs *ResourceSchema) bool {
	prop, ok := rs.Properties["tags"]
	if !ok {
		return false
	}
	return !prop.ReadOnly
}

// detectSupportsLocation checks if the resource has a writable "location" property.
func detectSupportsLocation(rs *ResourceSchema) bool {
	prop, ok := rs.Properties["location"]
	if !ok {
		return false
	}
	return !prop.ReadOnly
}

// detectSupportsIdentity checks if the resource supports managed identity configuration.
// This looks for writable "identity.type" or "identity.userAssignedIdentities" properties.
func detectSupportsIdentity(rs *ResourceSchema) bool {
	identityProp, ok := rs.Properties["identity"]
	if !ok || identityProp.ReadOnly {
		return false
	}

	if identityProp.Children == nil {
		return false
	}

	// Check for writable "type" sub-property
	typeProp, hasType := identityProp.Children["type"]
	if hasType && !typeProp.ReadOnly {
		return true
	}

	// Check for writable "userAssignedIdentities" sub-property
	uaiProp, hasUAI := identityProp.Children["userAssignedIdentities"]
	if hasUAI && !uaiProp.ReadOnly {
		return true
	}

	return false
}
