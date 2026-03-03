package schema

import (
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ptr is a helper to create a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}

func TestConvertResource_SimpleProperties(t *testing.T) {
	// Types array:
	// 0: StringType
	// 1: IntegerType
	// 2: BooleanType
	// 3: ObjectType (body) with name, count, enabled properties
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/simple@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringType{},  // 0
			&types.IntegerType{}, // 1
			&types.BooleanType{}, // 2
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/simple",
				Properties: map[string]types.ObjectTypeProperty{
					"name": {
						Type:        &types.TypeReference{Ref: 0},
						Flags:       types.TypePropertyFlagsRequired,
						Description: "The resource name",
					},
					"count": {
						Type:        &types.TypeReference{Ref: 1},
						Flags:       types.TypePropertyFlagsNone,
						Description: "The count",
					},
					"enabled": {
						Type:        &types.TypeReference{Ref: 2},
						Flags:       types.TypePropertyFlagsNone,
						Description: "Whether enabled",
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/simple",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)
	require.NotNil(t, rs)

	assert.Equal(t, "Microsoft.Test/simple", rs.ResourceType)
	assert.Equal(t, "2023-01-01", rs.APIVersion)
	assert.Len(t, rs.Properties, 3)

	// Check name property
	nameProp := rs.Properties["name"]
	require.NotNil(t, nameProp)
	assert.Equal(t, "name", nameProp.Name)
	assert.Equal(t, TypeString, nameProp.Type)
	assert.True(t, nameProp.Required)
	assert.False(t, nameProp.ReadOnly)
	assert.Equal(t, "The resource name", nameProp.Description)

	// Check count property
	countProp := rs.Properties["count"]
	require.NotNil(t, countProp)
	assert.Equal(t, TypeInteger, countProp.Type)
	assert.False(t, countProp.Required)
	assert.Equal(t, "The count", countProp.Description)

	// Check enabled property
	enabledProp := rs.Properties["enabled"]
	require.NotNil(t, enabledProp)
	assert.Equal(t, TypeBoolean, enabledProp.Type)
	assert.False(t, enabledProp.Required)
}

func TestConvertResource_NestedObjects(t *testing.T) {
	// Types array:
	// 0: StringType
	// 1: ObjectType (nested - "config" object)
	// 2: ObjectType (body) with "properties" -> nested object
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/nested@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (nested config object)
				Name: "Config",
				Properties: map[string]types.ObjectTypeProperty{
					"setting": {
						Type:        &types.TypeReference{Ref: 0},
						Flags:       types.TypePropertyFlagsRequired,
						Description: "A setting value",
					},
				},
			},
			&types.ObjectType{ // 2 (body)
				Name: "Microsoft.Test/nested",
				Properties: map[string]types.ObjectTypeProperty{
					"config": {
						Type:        &types.TypeReference{Ref: 1},
						Flags:       types.TypePropertyFlagsNone,
						Description: "Configuration block",
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/nested",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)
	require.NotNil(t, rs)

	configProp := rs.Properties["config"]
	require.NotNil(t, configProp)
	assert.Equal(t, TypeObject, configProp.Type)
	assert.Equal(t, "Configuration block", configProp.Description)
	require.NotNil(t, configProp.Children)
	assert.Len(t, configProp.Children, 1)

	settingProp := configProp.Children["setting"]
	require.NotNil(t, settingProp)
	assert.Equal(t, TypeString, settingProp.Type)
	assert.True(t, settingProp.Required)
	assert.Equal(t, "A setting value", settingProp.Description)
}

func TestConvertResource_ArrayType(t *testing.T) {
	// Types array:
	// 0: StringType (item type)
	// 1: ArrayType (with item type)
	// 2: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/arrays@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ArrayType{ // 1
				ItemType:  &types.TypeReference{Ref: 0},
				MinLength: ptr(int64(1)),
				MaxLength: ptr(int64(10)),
			},
			&types.ObjectType{ // 2
				Name: "Microsoft.Test/arrays",
				Properties: map[string]types.ObjectTypeProperty{
					"items": {
						Type:        &types.TypeReference{Ref: 1},
						Flags:       types.TypePropertyFlagsNone,
						Description: "List of items",
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/arrays",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	itemsProp := rs.Properties["items"]
	require.NotNil(t, itemsProp)
	assert.Equal(t, TypeArray, itemsProp.Type)
	assert.Equal(t, int64(1), *itemsProp.Constraints.MinItems)
	assert.Equal(t, int64(10), *itemsProp.Constraints.MaxItems)

	require.NotNil(t, itemsProp.ItemType)
	assert.Equal(t, TypeString, itemsProp.ItemType.Type)
}

func TestConvertResource_ArrayWithObjectItems(t *testing.T) {
	// Types array:
	// 0: StringType
	// 1: ObjectType (array item)
	// 2: ArrayType
	// 3: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/arrayobj@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (item object)
				Name: "ItemType",
				Properties: map[string]types.ObjectTypeProperty{
					"value": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired,
					},
				},
			},
			&types.ArrayType{ // 2
				ItemType: &types.TypeReference{Ref: 1},
			},
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/arrayobj",
				Properties: map[string]types.ObjectTypeProperty{
					"entries": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/arrayobj",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	entriesProp := rs.Properties["entries"]
	require.NotNil(t, entriesProp)
	assert.Equal(t, TypeArray, entriesProp.Type)

	require.NotNil(t, entriesProp.ItemType)
	assert.Equal(t, TypeObject, entriesProp.ItemType.Type)
	require.NotNil(t, entriesProp.ItemType.Children)
	assert.Len(t, entriesProp.ItemType.Children, 1)

	valueProp := entriesProp.ItemType.Children["value"]
	require.NotNil(t, valueProp)
	assert.Equal(t, TypeString, valueProp.Type)
	assert.True(t, valueProp.Required)
}

func TestConvertResource_DiscriminatedObject(t *testing.T) {
	// Types array:
	// 0: StringType
	// 1: IntegerType
	// 2: ObjectType (variant A)
	// 3: ObjectType (variant B)
	// 4: DiscriminatedObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/discriminated@2023-01-01",
			Body: &types.TypeReference{Ref: 4},
		},
		Types: []types.Type{
			&types.StringType{},  // 0
			&types.IntegerType{}, // 1
			&types.ObjectType{ // 2 (variant A)
				Name: "VariantA",
				Properties: map[string]types.ObjectTypeProperty{
					"aProp": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.ObjectType{ // 3 (variant B)
				Name: "VariantB",
				Properties: map[string]types.ObjectTypeProperty{
					"bProp": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.DiscriminatedObjectType{ // 4
				Name:          "Microsoft.Test/discriminated",
				Discriminator: "kind",
				BaseProperties: map[string]types.ObjectTypeProperty{
					"name": {
						Type:        &types.TypeReference{Ref: 0},
						Flags:       types.TypePropertyFlagsRequired,
						Description: "Resource name",
					},
				},
				Elements: map[string]types.ITypeReference{
					"A": &types.TypeReference{Ref: 2},
					"B": &types.TypeReference{Ref: 3},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/discriminated",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)
	require.NotNil(t, rs)

	// Should have: name (base), kind (discriminator), aProp (variant A), bProp (variant B)
	assert.GreaterOrEqual(t, len(rs.Properties), 3) // at least name, kind, and variant props

	// Discriminator property should be a required string enum
	kindProp := rs.Properties["kind"]
	require.NotNil(t, kindProp)
	assert.Equal(t, TypeString, kindProp.Type)
	assert.True(t, kindProp.Required)
	assert.Len(t, kindProp.Enum, 2)
	assert.Contains(t, kindProp.Enum, "A")
	assert.Contains(t, kindProp.Enum, "B")

	// Base property
	nameProp := rs.Properties["name"]
	require.NotNil(t, nameProp)
	assert.Equal(t, TypeString, nameProp.Type)
	assert.True(t, nameProp.Required)

	// Variant properties should not be required
	aProp := rs.Properties["aProp"]
	require.NotNil(t, aProp)
	assert.Equal(t, TypeString, aProp.Type)
	assert.False(t, aProp.Required)

	bProp := rs.Properties["bProp"]
	require.NotNil(t, bProp)
	assert.Equal(t, TypeInteger, bProp.Type)
	assert.False(t, bProp.Required)
}

func TestConvertResource_UnionTypeStringEnum(t *testing.T) {
	// Types array:
	// 0: StringLiteralType "Enabled"
	// 1: StringLiteralType "Disabled"
	// 2: UnionType [0, 1]
	// 3: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/unions@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringLiteralType{Value: "Enabled"},  // 0
			&types.StringLiteralType{Value: "Disabled"}, // 1
			&types.UnionType{ // 2
				Elements: []types.ITypeReference{
					&types.TypeReference{Ref: 0},
					&types.TypeReference{Ref: 1},
				},
			},
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/unions",
				Properties: map[string]types.ObjectTypeProperty{
					"status": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/unions",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	statusProp := rs.Properties["status"]
	require.NotNil(t, statusProp)
	assert.Equal(t, TypeString, statusProp.Type)
	assert.Len(t, statusProp.Enum, 2)
	assert.Contains(t, statusProp.Enum, "Enabled")
	assert.Contains(t, statusProp.Enum, "Disabled")
}

func TestConvertResource_NonStringUnion(t *testing.T) {
	// A union of non-string types should be treated as TypeAny
	// Types array:
	// 0: StringType
	// 1: IntegerType
	// 2: UnionType [0, 1] (mixed, not a string enum)
	// 3: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/mixedunion@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringType{},  // 0
			&types.IntegerType{}, // 1
			&types.UnionType{ // 2
				Elements: []types.ITypeReference{
					&types.TypeReference{Ref: 0},
					&types.TypeReference{Ref: 1},
				},
			},
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/mixedunion",
				Properties: map[string]types.ObjectTypeProperty{
					"value": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/mixedunion",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	valueProp := rs.Properties["value"]
	require.NotNil(t, valueProp)
	assert.Equal(t, TypeAny, valueProp.Type)
}

func TestConvertResource_SensitiveStringProperty(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/secrets@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{Sensitive: false}, // 0
			&types.StringType{Sensitive: true},  // 1
			&types.ObjectType{ // 2
				Name: "Microsoft.Test/secrets",
				Properties: map[string]types.ObjectTypeProperty{
					"name": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired,
					},
					"password": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsWriteOnly,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/secrets",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	nameProp := rs.Properties["name"]
	require.NotNil(t, nameProp)
	assert.False(t, nameProp.Sensitive)

	passwordProp := rs.Properties["password"]
	require.NotNil(t, passwordProp)
	assert.True(t, passwordProp.Sensitive)
	assert.True(t, passwordProp.WriteOnly)
}

func TestConvertResource_SensitiveObjectType(t *testing.T) {
	sensitiveTrue := true
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/sensitiveobj@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (sensitive object)
				Name:      "SensitiveConfig",
				Sensitive: &sensitiveTrue,
				Properties: map[string]types.ObjectTypeProperty{
					"key": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.ObjectType{ // 2
				Name: "Microsoft.Test/sensitiveobj",
				Properties: map[string]types.ObjectTypeProperty{
					"config": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/sensitiveobj",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	configProp := rs.Properties["config"]
	require.NotNil(t, configProp)
	assert.Equal(t, TypeObject, configProp.Type)
	assert.True(t, configProp.Sensitive)
}

func TestConvertResource_SensitiveStringLiteral(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/sensitivelit@2023-01-01",
			Body: &types.TypeReference{Ref: 1},
		},
		Types: []types.Type{
			&types.StringLiteralType{Value: "secret-value", Sensitive: true}, // 0
			&types.ObjectType{ // 1
				Name: "Microsoft.Test/sensitivelit",
				Properties: map[string]types.ObjectTypeProperty{
					"token": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/sensitivelit",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	tokenProp := rs.Properties["token"]
	require.NotNil(t, tokenProp)
	assert.Equal(t, TypeString, tokenProp.Type)
	assert.True(t, tokenProp.Sensitive)
	assert.Equal(t, []string{"secret-value"}, tokenProp.Enum)
}

func TestConvertResource_Constraints(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/constraints@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringType{ // 0
				MinLength: ptr(int64(3)),
				MaxLength: ptr(int64(128)),
				Pattern:   "^[a-z]+$",
			},
			&types.IntegerType{ // 1
				MinValue: ptr(int64(0)),
				MaxValue: ptr(int64(100)),
			},
			&types.ArrayType{ // 2
				ItemType:  &types.TypeReference{Ref: 0},
				MinLength: ptr(int64(1)),
				MaxLength: ptr(int64(50)),
			},
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/constraints",
				Properties: map[string]types.ObjectTypeProperty{
					"name": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired,
					},
					"priority": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
					"tags": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/constraints",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	// String constraints
	nameProp := rs.Properties["name"]
	require.NotNil(t, nameProp)
	assert.Equal(t, TypeString, nameProp.Type)
	require.NotNil(t, nameProp.Constraints.MinLength)
	assert.Equal(t, int64(3), *nameProp.Constraints.MinLength)
	require.NotNil(t, nameProp.Constraints.MaxLength)
	assert.Equal(t, int64(128), *nameProp.Constraints.MaxLength)
	assert.Equal(t, "^[a-z]+$", nameProp.Constraints.Pattern)

	// Integer constraints
	priorityProp := rs.Properties["priority"]
	require.NotNil(t, priorityProp)
	assert.Equal(t, TypeInteger, priorityProp.Type)
	require.NotNil(t, priorityProp.Constraints.MinValue)
	assert.Equal(t, int64(0), *priorityProp.Constraints.MinValue)
	require.NotNil(t, priorityProp.Constraints.MaxValue)
	assert.Equal(t, int64(100), *priorityProp.Constraints.MaxValue)

	// Array constraints
	tagsProp := rs.Properties["tags"]
	require.NotNil(t, tagsProp)
	assert.Equal(t, TypeArray, tagsProp.Type)
	require.NotNil(t, tagsProp.Constraints.MinItems)
	assert.Equal(t, int64(1), *tagsProp.Constraints.MinItems)
	require.NotNil(t, tagsProp.Constraints.MaxItems)
	assert.Equal(t, int64(50), *tagsProp.Constraints.MaxItems)
}

func TestConvertResource_SupportsTags(t *testing.T) {
	t.Run("writable tags property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/tagged@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.ObjectType{ // 0 (tags object)
					Name:                 "Tags",
					Properties:           map[string]types.ObjectTypeProperty{},
					AdditionalProperties: &types.TypeReference{Ref: 2},
				},
				&types.ObjectType{ // 1 (body)
					Name: "Microsoft.Test/tagged",
					Properties: map[string]types.ObjectTypeProperty{
						"tags": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
				&types.StringType{}, // 2
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/tagged",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.True(t, rs.SupportsTags)
	})

	t.Run("readonly tags property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/readonlytags@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.ObjectType{ // 0 (tags object)
					Name:       "Tags",
					Properties: map[string]types.ObjectTypeProperty{},
				},
				&types.ObjectType{ // 1
					Name: "Microsoft.Test/readonlytags",
					Properties: map[string]types.ObjectTypeProperty{
						"tags": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsReadOnly,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/readonlytags",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsTags)
	})

	t.Run("no tags property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/notags@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1
					Name: "Microsoft.Test/notags",
					Properties: map[string]types.ObjectTypeProperty{
						"name": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsRequired,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/notags",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsTags)
	})
}

func TestConvertResource_SupportsLocation(t *testing.T) {
	t.Run("writable location property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/located@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1
					Name: "Microsoft.Test/located",
					Properties: map[string]types.ObjectTypeProperty{
						"location": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsRequired,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/located",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.True(t, rs.SupportsLocation)
	})

	t.Run("readonly location property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/roLocation@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1
					Name: "Microsoft.Test/roLocation",
					Properties: map[string]types.ObjectTypeProperty{
						"location": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsReadOnly,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/roLocation",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsLocation)
	})
}

func TestConvertResource_SupportsIdentity(t *testing.T) {
	t.Run("with writable identity.type", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/identity@2023-01-01",
				Body: &types.TypeReference{Ref: 2},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1 (identity object)
					Name: "Identity",
					Properties: map[string]types.ObjectTypeProperty{
						"type": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
				&types.ObjectType{ // 2 (body)
					Name: "Microsoft.Test/identity",
					Properties: map[string]types.ObjectTypeProperty{
						"identity": {
							Type:  &types.TypeReference{Ref: 1},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/identity",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.True(t, rs.SupportsIdentity)
	})

	t.Run("with writable identity.userAssignedIdentities", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/uai@2023-01-01",
				Body: &types.TypeReference{Ref: 2},
			},
			Types: []types.Type{
				&types.ObjectType{ // 0 (UAI map)
					Name:       "UserAssignedIdentities",
					Properties: map[string]types.ObjectTypeProperty{},
				},
				&types.ObjectType{ // 1 (identity object)
					Name: "Identity",
					Properties: map[string]types.ObjectTypeProperty{
						"userAssignedIdentities": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
				&types.ObjectType{ // 2 (body)
					Name: "Microsoft.Test/uai",
					Properties: map[string]types.ObjectTypeProperty{
						"identity": {
							Type:  &types.TypeReference{Ref: 1},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/uai",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.True(t, rs.SupportsIdentity)
	})

	t.Run("readonly identity is not supported", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/roidentity@2023-01-01",
				Body: &types.TypeReference{Ref: 2},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1 (identity object)
					Name: "Identity",
					Properties: map[string]types.ObjectTypeProperty{
						"type": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
				&types.ObjectType{ // 2 (body)
					Name: "Microsoft.Test/roidentity",
					Properties: map[string]types.ObjectTypeProperty{
						"identity": {
							Type:  &types.TypeReference{Ref: 1},
							Flags: types.TypePropertyFlagsReadOnly,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/roidentity",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsIdentity)
	})

	t.Run("no identity property", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/noident@2023-01-01",
				Body: &types.TypeReference{Ref: 1},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1
					Name: "Microsoft.Test/noident",
					Properties: map[string]types.ObjectTypeProperty{
						"name": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsRequired,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/noident",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsIdentity)
	})

	t.Run("identity with only readonly sub-properties", func(t *testing.T) {
		loaded := &bicepdata.LoadedResource{
			ResourceType: &types.ResourceType{
				Name: "Microsoft.Test/rosubident@2023-01-01",
				Body: &types.TypeReference{Ref: 2},
			},
			Types: []types.Type{
				&types.StringType{}, // 0
				&types.ObjectType{ // 1 (identity object with readonly type)
					Name: "Identity",
					Properties: map[string]types.ObjectTypeProperty{
						"type": {
							Type:  &types.TypeReference{Ref: 0},
							Flags: types.TypePropertyFlagsReadOnly,
						},
					},
				},
				&types.ObjectType{ // 2 (body)
					Name: "Microsoft.Test/rosubident",
					Properties: map[string]types.ObjectTypeProperty{
						"identity": {
							Type:  &types.TypeReference{Ref: 1},
							Flags: types.TypePropertyFlagsNone,
						},
					},
				},
			},
			APIVersion:       "2023-01-01",
			ResourceTypeName: "Microsoft.Test/rosubident",
		}

		rs, err := ConvertResource(loaded)
		require.NoError(t, err)
		assert.False(t, rs.SupportsIdentity)
	})
}

func TestConvertResource_CycleDetection(t *testing.T) {
	// Create a circular reference: Object at index 1 references itself via a child property
	// Types array:
	// 0: StringType
	// 1: ObjectType (self-referencing)
	// 2: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/cyclic@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (self-referencing)
				Name: "Node",
				Properties: map[string]types.ObjectTypeProperty{
					"value": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
					"child": {
						Type:  &types.TypeReference{Ref: 1}, // self-reference
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.ObjectType{ // 2 (body)
				Name: "Microsoft.Test/cyclic",
				Properties: map[string]types.ObjectTypeProperty{
					"root": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/cyclic",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)
	require.NotNil(t, rs)

	rootProp := rs.Properties["root"]
	require.NotNil(t, rootProp)
	assert.Equal(t, TypeObject, rootProp.Type)
	require.NotNil(t, rootProp.Children)

	// First level should have "value" and "child"
	assert.Contains(t, rootProp.Children, "value")
	assert.Contains(t, rootProp.Children, "child")

	childProp := rootProp.Children["child"]
	require.NotNil(t, childProp)
	assert.Equal(t, TypeObject, childProp.Type)
	// Due to cycle detection, the nested "child" should have no children (cycle broken)
	assert.Nil(t, childProp.Children)
}

func TestConvertResource_PropertyFlags(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/flags@2023-01-01",
			Body: &types.TypeReference{Ref: 1},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1
				Name: "Microsoft.Test/flags",
				Properties: map[string]types.ObjectTypeProperty{
					"required": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired,
					},
					"readonly": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsReadOnly,
					},
					"writeonly": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsWriteOnly,
					},
					"deploytime": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsDeployTimeConstant,
					},
					"requiredReadonly": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired | types.TypePropertyFlagsReadOnly,
					},
					"none": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/flags",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	tests := []struct {
		name               string
		required           bool
		readOnly           bool
		writeOnly          bool
		deployTimeConstant bool
	}{
		{"required", true, false, false, false},
		{"readonly", false, true, false, false},
		{"writeonly", false, false, true, false},
		{"deploytime", false, false, false, true},
		{"requiredReadonly", true, true, false, false},
		{"none", false, false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prop := rs.Properties[tc.name]
			require.NotNil(t, prop, "property %q not found", tc.name)
			assert.Equal(t, tc.required, prop.Required, "Required mismatch")
			assert.Equal(t, tc.readOnly, prop.ReadOnly, "ReadOnly mismatch")
			assert.Equal(t, tc.writeOnly, prop.WriteOnly, "WriteOnly mismatch")
			assert.Equal(t, tc.deployTimeConstant, prop.DeployTimeConstant, "DeployTimeConstant mismatch")
		})
	}
}

func TestConvertResource_NilBody(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/nobody@2023-01-01",
			Body: nil,
		},
		Types:            []types.Type{},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/nobody",
	}

	_, err := ConvertResource(loaded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no body type reference")
}

func TestConvertResource_AnyType(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/any@2023-01-01",
			Body: &types.TypeReference{Ref: 1},
		},
		Types: []types.Type{
			&types.AnyType{}, // 0
			&types.ObjectType{ // 1
				Name: "Microsoft.Test/any",
				Properties: map[string]types.ObjectTypeProperty{
					"data": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/any",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	dataProp := rs.Properties["data"]
	require.NotNil(t, dataProp)
	assert.Equal(t, TypeAny, dataProp.Type)
}

func TestConvertResource_NullType(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/null@2023-01-01",
			Body: &types.TypeReference{Ref: 1},
		},
		Types: []types.Type{
			&types.NullType{}, // 0
			&types.ObjectType{ // 1
				Name: "Microsoft.Test/null",
				Properties: map[string]types.ObjectTypeProperty{
					"empty": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/null",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	emptyProp := rs.Properties["empty"]
	require.NotNil(t, emptyProp)
	assert.Equal(t, TypeNull, emptyProp.Type)
}

func TestConvertResource_NilPropertyType(t *testing.T) {
	// A property with nil Type should be treated as TypeAny
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/niltype@2023-01-01",
			Body: &types.TypeReference{Ref: 0},
		},
		Types: []types.Type{
			&types.ObjectType{ // 0
				Name: "Microsoft.Test/niltype",
				Properties: map[string]types.ObjectTypeProperty{
					"untyped": {
						Type:        nil,
						Flags:       types.TypePropertyFlagsNone,
						Description: "Untyped property",
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/niltype",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	untypedProp := rs.Properties["untyped"]
	require.NotNil(t, untypedProp)
	assert.Equal(t, TypeAny, untypedProp.Type)
}

func TestConvertResource_AdditionalProperties(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/addprops@2023-01-01",
			Body: &types.TypeReference{Ref: 2},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (map-like object)
				Name:                 "TagMap",
				Properties:           map[string]types.ObjectTypeProperty{},
				AdditionalProperties: &types.TypeReference{Ref: 0},
			},
			&types.ObjectType{ // 2 (body)
				Name: "Microsoft.Test/addprops",
				Properties: map[string]types.ObjectTypeProperty{
					"tags": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/addprops",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	tagsProp := rs.Properties["tags"]
	require.NotNil(t, tagsProp)
	assert.Equal(t, TypeObject, tagsProp.Type)
	require.NotNil(t, tagsProp.AdditionalProperties)
	assert.Equal(t, TypeString, tagsProp.AdditionalProperties.Type)
}

func TestConvertResource_StringLiteralType(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/literal@2023-01-01",
			Body: &types.TypeReference{Ref: 1},
		},
		Types: []types.Type{
			&types.StringLiteralType{Value: "fixedValue"}, // 0
			&types.ObjectType{ // 1
				Name: "Microsoft.Test/literal",
				Properties: map[string]types.ObjectTypeProperty{
					"constant": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/literal",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	constProp := rs.Properties["constant"]
	require.NotNil(t, constProp)
	assert.Equal(t, TypeString, constProp.Type)
	assert.Equal(t, []string{"fixedValue"}, constProp.Enum)
}

func TestConvertResource_NestedDiscriminatedObject(t *testing.T) {
	// A property (not body) that is a DiscriminatedObjectType
	// Types array:
	// 0: StringType
	// 1: ObjectType (variant "typeA")
	// 2: DiscriminatedObjectType (nested)
	// 3: ObjectType (body)
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/nesteddiscrim@2023-01-01",
			Body: &types.TypeReference{Ref: 3},
		},
		Types: []types.Type{
			&types.StringType{}, // 0
			&types.ObjectType{ // 1 (variant typeA)
				Name: "TypeA",
				Properties: map[string]types.ObjectTypeProperty{
					"aField": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.DiscriminatedObjectType{ // 2
				Name:           "NestedDiscrim",
				Discriminator:  "type",
				BaseProperties: map[string]types.ObjectTypeProperty{},
				Elements: map[string]types.ITypeReference{
					"typeA": &types.TypeReference{Ref: 1},
				},
			},
			&types.ObjectType{ // 3
				Name: "Microsoft.Test/nesteddiscrim",
				Properties: map[string]types.ObjectTypeProperty{
					"config": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/nesteddiscrim",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	configProp := rs.Properties["config"]
	require.NotNil(t, configProp)
	assert.Equal(t, TypeObject, configProp.Type)
	assert.Equal(t, "type", configProp.Discriminator)
	require.NotNil(t, configProp.Children)

	// Should have discriminator "type" as a string enum and "aField" from variant
	typeProp := configProp.Children["type"]
	require.NotNil(t, typeProp)
	assert.Equal(t, TypeString, typeProp.Type)
	assert.True(t, typeProp.Required)
	assert.Contains(t, typeProp.Enum, "typeA")

	aFieldProp := configProp.Children["aField"]
	require.NotNil(t, aFieldProp)
	assert.Equal(t, TypeString, aFieldProp.Type)
	assert.False(t, aFieldProp.Required) // variant props are not required
}

func TestConvertResource_BodyIsNotObjectType(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/badBody@2023-01-01",
			Body: &types.TypeReference{Ref: 0},
		},
		Types: []types.Type{
			&types.StringType{}, // 0 (body points here, which is wrong)
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/badBody",
	}

	_, err := ConvertResource(loaded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected body type")
}

func TestConvertResource_OutOfBoundsTypeRef(t *testing.T) {
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/oob@2023-01-01",
			Body: &types.TypeReference{Ref: 99}, // out of bounds
		},
		Types: []types.Type{
			&types.StringType{}, // 0
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/oob",
	}

	_, err := ConvertResource(loaded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestConvertResource_CompleteResource(t *testing.T) {
	// A realistic resource with location, tags, identity, and properties
	loaded := &bicepdata.LoadedResource{
		ResourceType: &types.ResourceType{
			Name: "Microsoft.Test/complete@2023-01-01",
			Body: &types.TypeReference{Ref: 6},
		},
		Types: []types.Type{
			&types.StringType{},  // 0
			&types.BooleanType{}, // 1
			&types.ObjectType{ // 2 (tags map)
				Name:                 "Tags",
				Properties:           map[string]types.ObjectTypeProperty{},
				AdditionalProperties: &types.TypeReference{Ref: 0},
			},
			&types.ObjectType{ // 3 (identity)
				Name: "Identity",
				Properties: map[string]types.ObjectTypeProperty{
					"type": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.ObjectType{ // 4 (properties object)
				Name: "Properties",
				Properties: map[string]types.ObjectTypeProperty{
					"featureEnabled": {
						Type:  &types.TypeReference{Ref: 1},
						Flags: types.TypePropertyFlagsNone,
					},
				},
			},
			&types.StringType{Sensitive: true}, // 5 (sensitive string)
			&types.ObjectType{ // 6 (body)
				Name: "Microsoft.Test/complete",
				Properties: map[string]types.ObjectTypeProperty{
					"name": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired | types.TypePropertyFlagsDeployTimeConstant,
					},
					"location": {
						Type:  &types.TypeReference{Ref: 0},
						Flags: types.TypePropertyFlagsRequired,
					},
					"tags": {
						Type:  &types.TypeReference{Ref: 2},
						Flags: types.TypePropertyFlagsNone,
					},
					"identity": {
						Type:  &types.TypeReference{Ref: 3},
						Flags: types.TypePropertyFlagsNone,
					},
					"properties": {
						Type:  &types.TypeReference{Ref: 4},
						Flags: types.TypePropertyFlagsNone,
					},
					"apiKey": {
						Type:  &types.TypeReference{Ref: 5},
						Flags: types.TypePropertyFlagsWriteOnly,
					},
				},
			},
		},
		APIVersion:       "2023-01-01",
		ResourceTypeName: "Microsoft.Test/complete",
	}

	rs, err := ConvertResource(loaded)
	require.NoError(t, err)

	assert.Equal(t, "Microsoft.Test/complete", rs.ResourceType)
	assert.Equal(t, "2023-01-01", rs.APIVersion)
	assert.True(t, rs.SupportsTags)
	assert.True(t, rs.SupportsLocation)
	assert.True(t, rs.SupportsIdentity)

	assert.Len(t, rs.Properties, 6)

	// name is required + deploy-time constant
	nameProp := rs.Properties["name"]
	require.NotNil(t, nameProp)
	assert.True(t, nameProp.Required)
	assert.True(t, nameProp.DeployTimeConstant)

	// apiKey is sensitive + write-only
	apiKeyProp := rs.Properties["apiKey"]
	require.NotNil(t, apiKeyProp)
	assert.True(t, apiKeyProp.Sensitive)
	assert.True(t, apiKeyProp.WriteOnly)
}

func TestTypeKind_String(t *testing.T) {
	tests := []struct {
		kind     TypeKind
		expected string
	}{
		{TypeString, "string"},
		{TypeInteger, "integer"},
		{TypeBoolean, "boolean"},
		{TypeArray, "array"},
		{TypeObject, "object"},
		{TypeAny, "any"},
		{TypeNull, "null"},
		{TypeKind(999), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.kind.String())
		})
	}
}

func TestProperty_IsScalar(t *testing.T) {
	tests := []struct {
		kind     TypeKind
		expected bool
	}{
		{TypeString, true},
		{TypeInteger, true},
		{TypeBoolean, true},
		{TypeNull, true},
		{TypeArray, false},
		{TypeObject, false},
		{TypeAny, false},
	}

	for _, tc := range tests {
		t.Run(tc.kind.String(), func(t *testing.T) {
			prop := &Property{Type: tc.kind}
			assert.Equal(t, tc.expected, prop.IsScalar())
		})
	}
}

func TestProperty_IsContainer(t *testing.T) {
	tests := []struct {
		kind     TypeKind
		expected bool
	}{
		{TypeObject, true},
		{TypeArray, true},
		{TypeString, false},
		{TypeInteger, false},
		{TypeBoolean, false},
		{TypeNull, false},
		{TypeAny, false},
	}

	for _, tc := range tests {
		t.Run(tc.kind.String(), func(t *testing.T) {
			prop := &Property{Type: tc.kind}
			assert.Equal(t, tc.expected, prop.IsContainer())
		})
	}
}
