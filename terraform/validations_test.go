package terraform

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func int64Ptr(v int64) *int64 { return &v }

func TestGenerateValidations_StringMinLength(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"displayName": {Name: "displayName", Type: schema.TypeString, Constraints: schema.Constraints{MinLength: int64Ptr(3)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"description": {Name: "description", Type: schema.TypeString, Constraints: schema.Constraints{MaxLength: int64Ptr(50)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

func TestGenerateValidations_StringPattern(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"resourceName": {Name: "resourceName", Type: schema.TypeString, Constraints: schema.Constraints{Pattern: "^[a-zA-Z0-9-_]{1,63}$"}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	nameVar := requireBlock(t, varsBody, "variable", "resource_name")

	validationBlock := findBlock(nameVar.Body, "validation")
	require.NotNil(t, validationBlock, "resource_name variable should have pattern validation")

	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "can(regex(")
	assert.Contains(t, conditionExpr, "^[a-zA-Z0-9-_]{1,63}$")
	assert.Contains(t, conditionExpr, "var.resource_name")

	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "must match the pattern")
	assert.Contains(t, errorMsg, "^[a-zA-Z0-9-_]{1,63}$")
}

func TestGenerateValidations_ArrayMinItems(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"tags": {Name: "tags", Type: schema.TypeArray, ItemType: &schema.Property{Type: schema.TypeString}, Constraints: schema.Constraints{MinItems: int64Ptr(1)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"items": {Name: "items", Type: schema.TypeArray, ItemType: &schema.Property{Type: schema.TypeString}, Constraints: schema.Constraints{MaxItems: int64Ptr(10)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

func TestGenerateValidations_NumberMinimum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"count": {Name: "count", Type: schema.TypeInteger, Constraints: schema.Constraints{MinValue: int64Ptr(1)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"percentage": {Name: "percentage", Type: schema.TypeInteger, Constraints: schema.Constraints{MaxValue: int64Ptr(100)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

func TestGenerateValidations_MultipleConstraints(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"username": {Name: "username", Type: schema.TypeString, Constraints: schema.Constraints{MinLength: int64Ptr(3), MaxLength: int64Ptr(100)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"requiredName": {Name: "requiredName", Type: schema.TypeString, Required: true, Constraints: schema.Constraints{MinLength: int64Ptr(1)}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
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

func TestGenerateValidations_Enum(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				"licenseType": {Name: "licenseType", Type: schema.TypeString, Enum: []string{"None", "Windows_Server"}},
			}},
		},
	}

	err = Generate("testResource", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	ltVar := requireBlock(t, varsBody, "variable", "license_type")

	validationBlock := findBlock(ltVar.Body, "validation")
	require.NotNil(t, validationBlock, "license_type variable should have enum validation")

	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "contains(")

	errorMsg := attributeStringValue(t, validationBlock.Body.Attributes["error_message"])
	assert.Contains(t, errorMsg, "None")
	assert.Contains(t, errorMsg, "Windows_Server")
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
