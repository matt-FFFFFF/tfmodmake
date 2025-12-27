package terraform

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
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

	maxLen := uint64(100)
	maxItems := uint64(5)
	min := float64(1)
	max := float64(1000)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"location": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
			"properties": {
				Value: &openapi3.Schema{
					Type:     &openapi3.Types{"object"},
					Required: []string{"sku"},
					Properties: map[string]*openapi3.SchemaRef{
						// Enum with allOf (simulating Azure patterns)
						"sku": {
							Value: &openapi3.Schema{
								AllOf: []*openapi3.SchemaRef{
									{
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"string"},
										},
									},
									{
										Value: &openapi3.Schema{
											Enum: []any{"Free", "Basic", "Standard", "Premium"},
										},
									},
								},
							},
						},
						// String with min/max length
						"resourceName": {
							Value: &openapi3.Schema{
								Type:      &openapi3.Types{"string"},
								MinLength: 3,
								MaxLength: &maxLen,
							},
						},
						// UUID format
						"correlationId": {
							Value: &openapi3.Schema{
								Type:   &openapi3.Types{"string"},
								Format: "uuid",
							},
						},
						// Array with constraints
						"allowedIpRanges": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"array"},
								MinItems:    1,
								MaxItems:    &maxItems,
								UniqueItems: true,
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
								},
							},
						},
						// Number with min/max
						"capacity": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
								Min:  &min,
								Max:  &max,
							},
						},
						// Enum via x-ms-enum
						"tier": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
								Extensions: map[string]any{
									"x-ms-enum": map[string]any{
										"name": "TierLevel",
										"values": []any{
											map[string]any{"value": "Development"},
											map[string]any{"value": "Production"},
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

	err = Generate(schema, "Microsoft.Test/comprehensive", "resource_body", "2024-01-01", false, false, nil)
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

	// Verify correlation_id UUID validation
	assert.Contains(t, varsContent, `variable "correlation_id"`)
	assert.Contains(t, varsContent, "can(regex(")
	assert.Contains(t, varsContent, "valid UUID")

	// Verify allowed_ip_ranges array validations
	assert.Contains(t, varsContent, `variable "allowed_ip_ranges"`)
	assert.Regexp(t, `length\(var\.allowed_ip_ranges\)\s+>=\s+1`, varsContent)
	assert.Regexp(t, `length\(var\.allowed_ip_ranges\)\s+<=\s+5`, varsContent)
	assert.Regexp(t, `length\(distinct\(var\.allowed_ip_ranges\)\)\s+==\s+length\(var\.allowed_ip_ranges\)`, varsContent)
	assert.Contains(t, varsContent, "unique items")

	// Verify capacity numeric validations
	assert.Contains(t, varsContent, `variable "capacity"`)
	assert.Regexp(t, `var\.capacity\s+>=\s+1`, varsContent)
	assert.Regexp(t, `var\.capacity\s+<=\s+1000`, varsContent)
	assert.Contains(t, varsContent, "greater than or equal to 1")
	assert.Contains(t, varsContent, "less than or equal to 1000")

	// Verify tier x-ms-enum validation (sorted)
	assert.Contains(t, varsContent, `variable "tier"`)
	assert.Contains(t, varsContent, `contains(["Development", "Production"], var.tier)`)

	// Verify null-safety for optional fields
	optionalVars := []string{"resource_name", "correlation_id", "allowed_ip_ranges", "capacity", "tier"}
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
	// - correlation_id: 1 (uuid)
	// - allowed_ip_ranges: 3 (min, max, unique)
	// - capacity: 2 (min, max)
	// - tier: 1 (x-ms-enum)
	// Total: 10
	assert.Equal(t, 10, validationCount, "Should have 10 validation blocks")

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
