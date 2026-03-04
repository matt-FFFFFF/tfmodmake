// Package schema provides an internal property/schema representation
// that serves as an adapter between bicep-types and tfmodmake's generation logic.
package schema

// TypeKind represents the kind of a property's type.
type TypeKind int

const (
	// TypeString represents a string type.
	TypeString TypeKind = iota
	// TypeInteger represents an integer/number type.
	TypeInteger
	// TypeBoolean represents a boolean type.
	TypeBoolean
	// TypeArray represents an array/list type.
	TypeArray
	// TypeObject represents an object type with named properties.
	TypeObject
	// TypeAny represents an untyped/dynamic value.
	TypeAny
	// TypeNull represents a null type.
	TypeNull
)

// String returns the string representation of a TypeKind.
func (tk TypeKind) String() string {
	switch tk {
	case TypeString:
		return "string"
	case TypeInteger:
		return "integer"
	case TypeBoolean:
		return "boolean"
	case TypeArray:
		return "array"
	case TypeObject:
		return "object"
	case TypeAny:
		return "any"
	case TypeNull:
		return "null"
	default:
		return "unknown"
	}
}

// Constraints holds validation constraints for a property.
type Constraints struct {
	// String constraints
	MinLength *int64
	MaxLength *int64
	Pattern   string

	// Numeric constraints
	MinValue *int64
	MaxValue *int64

	// Array constraints
	MinItems *int64
	MaxItems *int64
}

// Property represents a single property in a resource schema.
type Property struct {
	// Name is the property name as it appears in the ARM API.
	Name string

	// Description is the human-readable description of the property.
	Description string

	// Type is the kind of this property's value.
	Type TypeKind

	// Required indicates whether this property must be specified.
	Required bool

	// ReadOnly indicates that this property is set by the server and cannot be written.
	ReadOnly bool

	// WriteOnly indicates that this property can be written but is never returned in responses.
	WriteOnly bool

	// Sensitive indicates that this property contains a secret value.
	Sensitive bool

	// DeployTimeConstant indicates that this property can only be set at creation time.
	DeployTimeConstant bool

	// Children holds nested properties for object types.
	// Only populated when Type == TypeObject.
	Children map[string]*Property

	// ItemType holds the element type for array properties.
	// Only populated when Type == TypeArray.
	ItemType *Property

	// Enum holds allowed string values for enum-like properties.
	Enum []string

	// Constraints holds validation constraints.
	Constraints Constraints

	// AdditionalProperties indicates the type of additional properties for map-like objects.
	// nil means no additional properties are allowed.
	AdditionalProperties *Property

	// Discriminator is the property name used to discriminate between object variants.
	// Only set on properties that represent discriminated objects.
	Discriminator string
}

// HasDiscriminator reports whether the resource schema contains any
// discriminated object type at any nesting level. This is used to disable
// azapi embedded schema validation, which rejects unknown discriminator
// values during terraform validate when variables have not yet been assigned.
func HasDiscriminator(rs *ResourceSchema) bool {
	if rs == nil {
		return false
	}
	return hasDiscriminatorInProperties(rs.Properties)
}

func hasDiscriminatorInProperties(props map[string]*Property) bool {
	for _, prop := range props {
		if prop == nil {
			continue
		}
		if prop.Discriminator != "" {
			return true
		}
		if prop.Type == TypeObject && len(prop.Children) > 0 {
			if hasDiscriminatorInProperties(prop.Children) {
				return true
			}
		}
		if prop.Type == TypeArray && prop.ItemType != nil && prop.ItemType.Type == TypeObject {
			if hasDiscriminatorInProperties(prop.ItemType.Children) {
				return true
			}
		}
	}
	return false
}

// IsScalar returns true if the property represents a scalar (leaf) value.
func (p *Property) IsScalar() bool {
	switch p.Type {
	case TypeString, TypeInteger, TypeBoolean, TypeNull:
		return true
	default:
		return false
	}
}

// IsContainer returns true if the property represents a container (object or array).
func (p *Property) IsContainer() bool {
	return p.Type == TypeObject || p.Type == TypeArray
}

// ResourceSchema represents the fully resolved schema for an Azure resource type.
type ResourceSchema struct {
	// Properties holds the top-level properties of the resource body.
	Properties map[string]*Property

	// ResourceType is the fully qualified Azure resource type (e.g. "Microsoft.App/containerApps").
	ResourceType string

	// APIVersion is the API version (e.g. "2025-01-01").
	APIVersion string

	// SupportsTags indicates whether the resource has a writable "tags" property.
	SupportsTags bool

	// SupportsLocation indicates whether the resource has a writable "location" property.
	SupportsLocation bool

	// SupportsIdentity indicates whether the resource supports managed identity configuration.
	SupportsIdentity bool
}
