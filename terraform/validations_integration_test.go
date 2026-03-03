package terraform

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ComprehensiveValidations tests all validation types together
// in a realistic scenario similar to Azure resource schemas.
func TestIntegration_ComprehensiveValidations(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	rs := &schema.ResourceSchema{
		Properties: map[string]*schema.Property{
			"location": {Name: "location", Type: schema.TypeString},
			"properties": {Name: "properties", Type: schema.TypeObject, Children: map[string]*schema.Property{
				// Enum (required field)
				"sku": {Name: "sku", Type: schema.TypeString, Required: true, Enum: []string{"Free", "Basic", "Standard", "Premium"}},
				// String with min/max length
				"resourceName": {Name: "resourceName", Type: schema.TypeString, Constraints: schema.Constraints{MinLength: int64Ptr(3), MaxLength: int64Ptr(100)}},
				// Array with constraints
				"allowedIpRanges": {Name: "allowedIpRanges", Type: schema.TypeArray, ItemType: &schema.Property{Type: schema.TypeString}, Constraints: schema.Constraints{MinItems: int64Ptr(1), MaxItems: int64Ptr(5)}},
				// Number with min/max
				"capacity": {Name: "capacity", Type: schema.TypeInteger, Constraints: schema.Constraints{MinValue: int64Ptr(1), MaxValue: int64Ptr(1000)}},
				// Enum (optional field)
				"tier": {Name: "tier", Type: schema.TypeString, Enum: []string{"Development", "Production"}},
			}},
		},
	}

	err = Generate("Microsoft.Test/comprehensive", WithResourceSchema(rs), WithLocalName("resource_body"), WithAPIVersion("2024-01-01"))
	require.NoError(t, err)

	// Read and verify the generated file
	varsBytes, err := os.ReadFile("variables.tf")
	require.NoError(t, err)
	varsContent := string(varsBytes)

	// Verify SKU enum validation (required field, sorted enum values)
	assert.Contains(t, varsContent, `variable "sku"`)
	assert.Contains(t, varsContent, `contains(["Basic", "Free", "Premium", "Standard"], var.sku)`)
	assert.NotContains(t, varsContent, "var.sku == null") // Required field shouldn't have null check

	// Verify resource_name string validations
	assert.Contains(t, varsContent, `variable "resource_name"`)
	assert.Regexp(t, `length\(var\.resource_name\)\s+>=\s+3`, varsContent)
	assert.Regexp(t, `length\(var\.resource_name\)\s+<=\s+100`, varsContent)
	assert.Contains(t, varsContent, "minimum length of 3")
	assert.Contains(t, varsContent, "maximum length of 100")

	// Verify allowed_ip_ranges array validations
	assert.Contains(t, varsContent, `variable "allowed_ip_ranges"`)
	assert.Regexp(t, `length\(var\.allowed_ip_ranges\)\s+>=\s+1`, varsContent)
	assert.Regexp(t, `length\(var\.allowed_ip_ranges\)\s+<=\s+5`, varsContent)

	// Verify capacity numeric validations
	assert.Contains(t, varsContent, `variable "capacity"`)
	assert.Regexp(t, `var\.capacity\s+>=\s+1`, varsContent)
	assert.Regexp(t, `var\.capacity\s+<=\s+1000`, varsContent)
	assert.Contains(t, varsContent, "greater than or equal to 1")
	assert.Contains(t, varsContent, "less than or equal to 1000")

	// Verify tier enum validation (sorted)
	assert.Contains(t, varsContent, `variable "tier"`)
	assert.Contains(t, varsContent, `contains(["Development", "Production"], var.tier)`)

	// Verify null-safety for optional fields
	optionalVars := []string{"resource_name", "allowed_ip_ranges", "capacity", "tier"}
	for _, varName := range optionalVars {
		// Each optional variable should have at least one validation with null check (allowing extra spaces)
		varBlock := extractVariableBlock(t, varsContent, varName)
		if strings.Contains(varBlock, "validation {") {
			// Use regex to match with flexible whitespace
			pattern := `var\.` + strings.ReplaceAll(varName, "_", "_") + `\s+==\s+null\s+\|\|`
			matched, _ := regexp.MatchString(pattern, varBlock)
			assert.True(t, matched,
				"%s should have null-safe validation (pattern: %s)", varName, pattern)
		}
	}

	// Count total validations
	validationCount := strings.Count(varsContent, "validation {")
	// Expected:
	// - sku: 1 (enum)
	// - resource_name: 2 (min, max)
	// - allowed_ip_ranges: 2 (min, max)
	// - capacity: 2 (min, max)
	// - tier: 1 (enum)
	// Total: 8
	assert.Equal(t, 8, validationCount, "Should have 8 validation blocks")

	t.Logf("Generated %d validation blocks", validationCount)
}

// extractVariableBlock extracts the content of a specific variable block from the file
func extractVariableBlock(t *testing.T, content, varName string) string {
	t.Helper()

	start := strings.Index(content, `variable "`+varName+`"`)
	if start == -1 {
		return ""
	}

	// Find the closing brace
	braceCount := 0
	inBlock := false
	end := start
	for i := start; i < len(content); i++ {
		if content[i] == '{' {
			braceCount++
			inBlock = true
		} else if content[i] == '}' {
			braceCount--
			if inBlock && braceCount == 0 {
				end = i + 1
				break
			}
		}
	}

	if end > start {
		return content[start:end]
	}
	return ""
}
