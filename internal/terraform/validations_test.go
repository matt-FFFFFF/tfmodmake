package terraform

import (
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateValidations_StringMinLength(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"displayName": {
							Value: &openapi3.Schema{
								Type:      &openapi3.Types{"string"},
								MinLength: 3,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	nameVar := requireBlock(t, varsBody, "variable", "display_name")
	
	validationBlock := findBlock(nameVar.Body, "validation")
	require.NotNil(t, validationBlock, "display_name variable should have minLength validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.display_name == null || length(var.display_name) >= 3")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "minimum length of 3")
}

func TestGenerateValidations_StringMaxLength(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	maxLen := uint64(50)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"description": {
							Value: &openapi3.Schema{
								Type:      &openapi3.Types{"string"},
								MaxLength: &maxLen,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	descVar := requireBlock(t, varsBody, "variable", "description")
	
	validationBlock := findBlock(descVar.Body, "validation")
	require.NotNil(t, validationBlock, "description variable should have maxLength validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.description == null || length(var.description) <= 50")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "maximum length of 50")
}

func TestGenerateValidations_StringUUIDFormat(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"id": {
							Value: &openapi3.Schema{
								Type:   &openapi3.Types{"string"},
								Format: "uuid",
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	idVar := requireBlock(t, varsBody, "variable", "id")
	
	validationBlock := findBlock(idVar.Body, "validation")
	require.NotNil(t, validationBlock, "id variable should have UUID format validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "can(regex(")
	assert.Contains(t, conditionExpr, "var.id")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "valid UUID")
}

func TestGenerateValidations_ArrayMinItems(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"tags": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"array"},
								MinItems: 1,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	tagsVar := requireBlock(t, varsBody, "variable", "tags")
	
	validationBlock := findBlock(tagsVar.Body, "validation")
	require.NotNil(t, validationBlock, "tags variable should have minItems validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.tags == null || length(var.tags) >= 1")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "at least 1 item")
}

func TestGenerateValidations_ArrayMaxItems(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	maxItems := uint64(10)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"items": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"array"},
								MaxItems: &maxItems,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	itemsVar := requireBlock(t, varsBody, "variable", "items")
	
	validationBlock := findBlock(itemsVar.Body, "validation")
	require.NotNil(t, validationBlock, "items variable should have maxItems validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.items == null || length(var.items) <= 10")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "at most 10 item")
}

func TestGenerateValidations_ArrayUniqueItems(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"uniqueList": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"array"},
								UniqueItems: true,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	uniqueVar := requireBlock(t, varsBody, "variable", "unique_list")
	
	validationBlock := findBlock(uniqueVar.Body, "validation")
	require.NotNil(t, validationBlock, "uniqueList variable should have uniqueItems validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "length(distinct(var.unique_list)) == length(var.unique_list)")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "unique items")
}

func TestGenerateValidations_NumberMinimum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	min := float64(1)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"count": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
								Min:  &min,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	countVar := requireBlock(t, varsBody, "variable", "count")
	
	validationBlock := findBlock(countVar.Body, "validation")
	require.NotNil(t, validationBlock, "count variable should have minimum validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.count == null || var.count >= 1")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "greater than or equal to 1")
}

func TestGenerateValidations_NumberMaximum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	max := float64(100)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"percentage": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"number"},
								Max:  &max,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	percentVar := requireBlock(t, varsBody, "variable", "percentage")
	
	validationBlock := findBlock(percentVar.Body, "validation")
	require.NotNil(t, validationBlock, "percentage variable should have maximum validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.percentage == null || var.percentage <= 100")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "less than or equal to 100")
}

func TestGenerateValidations_NumberExclusiveMinimum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	min := float64(0)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"rating": {
							Value: &openapi3.Schema{
								Type:         &openapi3.Types{"number"},
								Min:          &min,
								ExclusiveMin: true,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	ratingVar := requireBlock(t, varsBody, "variable", "rating")
	
	validationBlock := findBlock(ratingVar.Body, "validation")
	require.NotNil(t, validationBlock, "rating variable should have exclusive minimum validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.rating == null || var.rating > 0")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "greater than 0")
}

func TestGenerateValidations_NumberExclusiveMaximum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	max := float64(10)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"scale": {
							Value: &openapi3.Schema{
								Type:         &openapi3.Types{"integer"},
								Max:          &max,
								ExclusiveMax: true,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	scaleVar := requireBlock(t, varsBody, "variable", "scale")
	
	validationBlock := findBlock(scaleVar.Body, "validation")
	require.NotNil(t, validationBlock, "scale variable should have exclusive maximum validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.scale == null || var.scale < 10")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "less than 10")
}

func TestGenerateValidations_NumberMultipleOf(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	multipleOf := float64(5)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"size": {
							Value: &openapi3.Schema{
								Type:       &openapi3.Types{"integer"},
								MultipleOf: &multipleOf,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	sizeVar := requireBlock(t, varsBody, "variable", "size")
	
	validationBlock := findBlock(sizeVar.Body, "validation")
	require.NotNil(t, validationBlock, "size variable should have multipleOf validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "abs(mod(var.size, 5))")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "multiple of 5")
}

func TestGenerateValidations_EnumViaAllOf(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"tier": {
							Value: &openapi3.Schema{
								AllOf: []*openapi3.SchemaRef{
									{
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"string"},
										},
									},
									{
										Value: &openapi3.Schema{
											Enum: []any{"Basic", "Standard", "Premium"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	tierVar := requireBlock(t, varsBody, "variable", "tier")
	
	validationBlock := findBlock(tierVar.Body, "validation")
	require.NotNil(t, validationBlock, "tier variable should have enum validation via allOf")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "contains(")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	// Enum values should be sorted alphabetically
	assert.Contains(t, errorMsg, "Basic")
	assert.Contains(t, errorMsg, "Premium")
	assert.Contains(t, errorMsg, "Standard")
}

func TestGenerateValidations_XMsEnum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"sku": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
								Extensions: map[string]any{
									"x-ms-enum": map[string]any{
										"name": "SkuName",
										"values": []any{
											map[string]any{"value": "Free"},
											map[string]any{"value": "Shared"},
											map[string]any{"value": "Basic"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	skuVar := requireBlock(t, varsBody, "variable", "sku")
	
	validationBlock := findBlock(skuVar.Body, "validation")
	require.NotNil(t, validationBlock, "sku variable should have x-ms-enum validation")
	
	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	// Enum values should be sorted
	assert.Contains(t, errorMsg, "Basic")
	assert.Contains(t, errorMsg, "Free")
	assert.Contains(t, errorMsg, "Shared")
}

func TestGenerateValidations_MultipleConstraints(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	maxLen := uint64(100)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"username": {
							Value: &openapi3.Schema{
								Type:      &openapi3.Types{"string"},
								MinLength: 3,
								MaxLength: &maxLen,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	usernameVar := requireBlock(t, varsBody, "variable", "username")
	
	// Should have both minLength and maxLength validations
	validationBlocks := findAllBlocks(usernameVar.Body, "validation")
	require.Len(t, validationBlocks, 2, "username should have 2 validations (minLength and maxLength)")
}

func TestGenerateValidations_RequiredField(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type:     &openapi3.Types{"object"},
					Required: []string{"requiredName"},
					Properties: map[string]*openapi3.SchemaRef{
						"requiredName": {
							Value: &openapi3.Schema{
								Type:      &openapi3.Types{"string"},
								MinLength: 1,
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	reqVar := requireBlock(t, varsBody, "variable", "required_name")
	
	// Should NOT have default = null
	assert.Nil(t, reqVar.Body.Attributes["default"], "required variable should not have default")
	
	validationBlock := findBlock(reqVar.Body, "validation")
	require.NotNil(t, validationBlock, "required variable should still have validation")
	
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	// Required fields should NOT have null check
	assert.NotContains(t, conditionExpr, "== null ||", "required field should not have null check")
	assert.Contains(t, conditionExpr, "length(var.required_name) >= 1")
}

func TestResolveSchemaForValidation_AllOfMostRestrictive(t *testing.T) {
	maxLen100 := uint64(100)
	maxLen50 := uint64(50)
	maxItems10 := uint64(10)
	maxItems5 := uint64(5)
	min0 := float64(0)
	min5 := float64(5)
	max20 := float64(20)
	max10 := float64(10)

	s := &openapi3.Schema{
		AllOf: []*openapi3.SchemaRef{
			{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, MinLength: 1, MaxLength: &maxLen100}},
			{Value: &openapi3.Schema{MinLength: 3, MaxLength: &maxLen50}},
			{Value: &openapi3.Schema{MinItems: 1, MaxItems: &maxItems10}},
			{Value: &openapi3.Schema{MinItems: 2, MaxItems: &maxItems5}},
			{Value: &openapi3.Schema{Min: &min0, Max: &max20}},
			{Value: &openapi3.Schema{Min: &min5, ExclusiveMin: true, Max: &max10, ExclusiveMax: true}},
		},
	}

	resolved := resolveSchemaForValidation(s)
	require.NotNil(t, resolved)
	assert.Equal(t, uint64(3), resolved.MinLength)
	if assert.NotNil(t, resolved.MaxLength) {
		assert.Equal(t, uint64(50), *resolved.MaxLength)
	}
	assert.Equal(t, uint64(2), resolved.MinItems)
	if assert.NotNil(t, resolved.MaxItems) {
		assert.Equal(t, uint64(5), *resolved.MaxItems)
	}
	if assert.NotNil(t, resolved.Min) {
		assert.Equal(t, 5.0, *resolved.Min)
		assert.True(t, resolved.ExclusiveMin)
	}
	if assert.NotNil(t, resolved.Max) {
		assert.Equal(t, 10.0, *resolved.Max)
		assert.True(t, resolved.ExclusiveMax)
	}
}

// Helper function to find all blocks of a given type
func findAllBlocks(body *hclsyntax.Body, typ string) []*hclsyntax.Block {
	var blocks []*hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == typ {
			blocks = append(blocks, block)
		}
	}
	return blocks
}
