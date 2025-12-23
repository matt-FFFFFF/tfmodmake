package terraform

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"camelCase", "camel_case"},
		{"PascalCase", "pascal_case"},
		{"snake_case", "snake_case"},
		{"HTTPClient", "http_client"},
		{"simple", "simple"},
		{"agentPoolProfiles", "agent_pool_profiles"},
		{"AdminGroupObjectIDs", "admin_group_object_ids"},
		{"HTTPServer", "http_server"},
		{"JSONList", "json_list"},
		{"MyAPIs", "my_apis"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSnakeCase(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerate(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"location": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"string"},
					Description: "Resource location",
				},
			},
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"readOnlyProp": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"string"},
								ReadOnly: true,
							},
						},
						"writableProp": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
							},
						},
					},
				},
			},
		},
	}

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)

	apiVersion := "2024-01-01"
	err = Generate(schema, "testResource", "test_local", apiVersion, supportsTags, supportsLocation)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")

	nameVar := requireBlock(t, varsBody, "variable", "name")
	assert.Equal(t, "The name of the resource.\n", attributeStringValue(t, nameVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, nameVar.Body.Attributes["type"].Expr))

	parentVar := requireBlock(t, varsBody, "variable", "parent_id")
	assert.Equal(t, "The parent resource ID for this resource.\n", attributeStringValue(t, parentVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, parentVar.Body.Attributes["type"].Expr))

	assert.Nil(t, findBlock(varsBody, "variable", "tags"))

	locationVar := requireBlock(t, varsBody, "variable", "location")
	require.NotNil(t, locationVar)
	assert.Equal(t, "The location of the resource.\n", attributeStringValue(t, locationVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, locationVar.Body.Attributes["type"].Expr))

	propertiesVar := requireBlock(t, varsBody, "variable", "properties")
	desc := attributeStringValue(t, propertiesVar.Body.Attributes["description"])
	assert.Contains(t, desc, "The properties of the resource.")
	assert.Contains(t, desc, "writable_prop")
	typeExpr := expressionString(t, propertiesVar.Body.Attributes["type"].Expr)
	assert.Contains(t, typeExpr, "writable_prop")
	assert.NotContains(t, typeExpr, "read_only_prop")
	assert.Equal(t, "null", expressionString(t, propertiesVar.Body.Attributes["default"].Expr))

	localsBody := parseHCLBody(t, "locals.tf")
	localsBlock := requireBlock(t, localsBody, "locals")
	localAttr := localsBlock.Body.Attributes["test_local"]
	localExpr := expressionString(t, localAttr.Expr)
	assert.Contains(t, localExpr, "location = var.location")
	assert.Contains(t, localExpr, "writableProp = var.properties.writable_prop")
	assert.NotContains(t, localExpr, "readOnlyProp")

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	assert.Equal(t, "testResource@2024-01-01", attributeStringValue(t, resourceBlock.Body.Attributes["type"]))
	assert.Equal(t, "var.name", expressionString(t, resourceBlock.Body.Attributes["name"].Expr))
	assert.Equal(t, "var.parent_id", expressionString(t, resourceBlock.Body.Attributes["parent_id"].Expr))
	assert.Equal(t, "var.location", expressionString(t, resourceBlock.Body.Attributes["location"].Expr))
	bodyExpr := expressionString(t, resourceBlock.Body.Attributes["body"].Expr)
	assert.Contains(t, bodyExpr, "properties = local.test_local")
	assert.Nil(t, resourceBlock.Body.Attributes["tags"])

	outputsBody := parseHCLBody(t, "outputs.tf")
	idOutput := requireBlock(t, outputsBody, "output", "resource_id")
	assert.Equal(t, "The ID of the created resource.", attributeStringValue(t, idOutput.Body.Attributes["description"]))
	assert.Equal(t, "azapi_resource.this.id", expressionString(t, idOutput.Body.Attributes["value"].Expr))

	nameOutput := requireBlock(t, outputsBody, "output", "name")
	assert.Equal(t, "The name of the created resource.", attributeStringValue(t, nameOutput.Body.Attributes["description"]))
	assert.Equal(t, "azapi_resource.this.name", expressionString(t, nameOutput.Body.Attributes["value"].Expr))
}

func TestGenerate_IncludesAdditionalPropertiesDescription(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"kubeDnsOverrides": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"object"},
					Description: "Overrides for kube DNS queries.",
					AdditionalProperties: openapi3.AdditionalProperties{
						Schema: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"queryLogging": {
										Value: &openapi3.Schema{
											Type:        &openapi3.Types{"string"},
											Description: "Enable query logging.",
										},
									},
									"maxConcurrent": {
										Value: &openapi3.Schema{
											Type:        &openapi3.Types{"integer"},
											Description: "Maximum concurrent queries.",
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

	supportsLocation := SupportsLocation(schema)

	err = Generate(schema, "testResource", "local_map", "", false, supportsLocation)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	overrideVar := requireBlock(t, varsBody, "variable", "kube_dns_overrides")
	desc := attributeStringValue(t, overrideVar.Body.Attributes["description"])
	assert.Contains(t, desc, "Overrides for kube DNS queries.")
	assert.Contains(t, desc, "Map values:")
	assert.Contains(t, desc, "- `max_concurrent` - Maximum concurrent queries.")
	assert.Contains(t, desc, "- `query_logging` - Enable query logging.")
}

func TestGenerate_WithTagsSupport(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"location": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			"tags": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					AdditionalProperties: openapi3.AdditionalProperties{
						Schema: &openapi3.SchemaRef{
							Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
						},
					},
				},
			},
		},
	}

	supportsLocation := SupportsLocation(schema)

	err = Generate(schema, "testResource", "resource_body", "", true, supportsLocation)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	tagsVar := requireBlock(t, varsBody, "variable", "tags")
	assert.Equal(t, "map(string)", expressionString(t, tagsVar.Body.Attributes["type"].Expr))
	assert.Equal(t, "null", expressionString(t, tagsVar.Body.Attributes["default"].Expr))

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	assert.Equal(t, "var.tags", expressionString(t, resourceBlock.Body.Attributes["tags"].Expr))
	assert.Equal(t, "var.location", expressionString(t, resourceBlock.Body.Attributes["location"].Expr))
}

func TestGenerate_UsesPlaceholderWhenVersionMissing(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"location": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
		},
	}

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)

	err = Generate(schema, "testResource", "placeholder_local", "", supportsTags, supportsLocation)
	require.NoError(t, err)

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	assert.Equal(t, "testResource@apiVersion", attributeStringValue(t, resourceBlock.Body.Attributes["type"]))
}

func TestGenerate_WithNilSchemaSetsEmptyBody(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	err = Generate(nil, "testResource", "unused_local", "2024-01-01", false, false)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	requireBlock(t, varsBody, "variable", "name")
	requireBlock(t, varsBody, "variable", "parent_id")

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	bodyExpr := expressionString(t, resourceBlock.Body.Attributes["body"].Expr)
	assert.Equal(t, "{}", bodyExpr)

	_, err = os.Stat("locals.tf")
	assert.True(t, os.IsNotExist(err))
}

func TestMapType(t *testing.T) {
	tests := []struct {
		name   string
		schema *openapi3.Schema
		want   string
	}{
		{
			name:   "string",
			schema: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			want:   "string",
		},
		{
			name:   "integer",
			schema: &openapi3.Schema{Type: &openapi3.Types{"integer"}},
			want:   "number",
		},
		{
			name:   "boolean",
			schema: &openapi3.Schema{Type: &openapi3.Types{"boolean"}},
			want:   "bool",
		},
		{
			name: "array of strings",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
			want: "list(string)",
		},
		{
			name: "object",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"prop1": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
				},
			},
			want: "object({\n  prop1 = optional(string)\n})",
		},
		{
			name: "object with additionalProperties object",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:     &openapi3.Types{"object"},
							Required: []string{"queryLogging"},
							Properties: map[string]*openapi3.SchemaRef{
								"queryLogging":  {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
								"maxConcurrent": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
							},
						},
					},
				},
			},
			want: "map(object({\n  max_concurrent = optional(number)\n  query_logging  = string\n}))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTokens := mapType(tt.schema)
			got := string(gotTokens.Bytes())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildNestedDescription(t *testing.T) {
	schema := &openapi3.Schema{
		Properties: map[string]*openapi3.SchemaRef{
			"prop1": {
				Value: &openapi3.Schema{
					Description: "Description 1",
				},
			},
			"nested": {
				Value: &openapi3.Schema{
					Type:        &openapi3.Types{"object"},
					Description: "Nested object",
					Properties: map[string]*openapi3.SchemaRef{
						"child": {
							Value: &openapi3.Schema{
								Description: "Child description",
							},
						},
					},
				},
			},
		},
	}

	got := buildNestedDescription(schema, "")
	assert.Contains(t, got, "- `prop1` - Description 1")
	assert.Contains(t, got, "- `nested` - Nested object")
	assert.Contains(t, got, "  - `child` - Child description")
}

func TestConstructValue_MapAdditionalPropertiesObject(t *testing.T) {
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"queryLogging": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
							},
						},
						"maxConcurrent": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
							},
						},
					},
				},
			},
		},
	}

	accessPath := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("var")},
		{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("kube_dns_overrides")},
	}
	tokens := constructValue(schema, accessPath, false, nil)

	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("attr", tokens)
	expected := `attr = var.kube_dns_overrides == null ? null : { for k, value in var.kube_dns_overrides : k => value == null ? null : {
  maxConcurrent = value.max_concurrent
  queryLogging  = value.query_logging
} }
`
	buf := new(bytes.Buffer)
	_, err := f.WriteTo(buf)
	require.NoError(t, err)
	parsed, diags := hclwrite.ParseConfig(buf.Bytes(), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	attr := parsed.Body().GetAttribute("attr")
	resultTokens := attr.BuildTokens(nil)
	assert.Equal(t, expected, string(resultTokens.Bytes()))
}

func parseHCLBody(t *testing.T, path string) *hclsyntax.Body {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	file, diags := hclsyntax.ParseConfig(data, path, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok, "expected hclsyntax.Body")

	return body
}

func findBlock(body *hclsyntax.Body, typ string, labels ...string) *hclsyntax.Block {
	for _, block := range body.Blocks {
		if block.Type != typ {
			continue
		}
		if len(labels) == 0 && len(block.Labels) == 0 {
			return block
		}
		if len(block.Labels) != len(labels) {
			continue
		}
		match := true
		for i, l := range labels {
			if block.Labels[i] != l {
				match = false
				break
			}
		}
		if match {
			return block
		}
	}
	return nil
}

func requireBlock(t *testing.T, body *hclsyntax.Body, typ string, labels ...string) *hclsyntax.Block {
	t.Helper()
	block := findBlock(body, typ, labels...)
	require.NotNil(t, block, "expected block %s %v", typ, labels)
	return block
}

func attributeStringValue(t *testing.T, attr *hclsyntax.Attribute) string {
	t.Helper()
	require.NotNil(t, attr)
	val, diags := attr.Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	require.True(t, val.Type().Equals(cty.String), "expected string value, got %s", val.Type().FriendlyName())
	return val.AsString()
}

func expressionString(t *testing.T, expr hcl.Expression) string {
	t.Helper()

	rng := expr.Range()
	data, err := os.ReadFile(rng.Filename)
	require.NoError(t, err)

	require.LessOrEqual(t, rng.End.Byte, len(data), "expression end out of range")
	require.LessOrEqual(t, rng.Start.Byte, rng.End.Byte, "expression range invalid")

	exprSrc := data[rng.Start.Byte:rng.End.Byte]
	formatted := hclwrite.Format(exprSrc)
	return strings.TrimSpace(string(formatted))
}

func TestGenerate_WithSecretFields(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create a schema with a secret field marked with x-ms-secret extension
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"normalField": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "A normal field",
							},
						},
						"connectionString": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "Application Insights connection string",
								Extensions: map[string]any{
									"x-ms-secret": true,
								},
							},
						},
						"apiKey": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "The API key",
								Extensions: map[string]any{
									"x-ms-secret": true,
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "Microsoft.Test/testResource", "resource_body", "2024-01-01", false, false)
	require.NoError(t, err)

	// Check variables.tf
	varsBody := parseHCLBody(t, "variables.tf")

	// Properties should be generated as a nested object variable
	propertiesVar := requireBlock(t, varsBody, "variable", "properties")
	assert.NotNil(t, propertiesVar)
	
	// Check that properties has the right type structure including both normal and secret fields
	typeExpr := expressionString(t, propertiesVar.Body.Attributes["type"].Expr)
	assert.Contains(t, typeExpr, "normal_field")
	assert.Contains(t, typeExpr, "connection_string")
	assert.Contains(t, typeExpr, "api_key")

	// Secret fields should also have separate top-level variables with ephemeral = true
	connectionStringVar := requireBlock(t, varsBody, "variable", "connection_string")
	assert.NotNil(t, connectionStringVar)
	assert.Equal(t, "string", expressionString(t, connectionStringVar.Body.Attributes["type"].Expr))
	ephemeralAttr := connectionStringVar.Body.Attributes["ephemeral"]
	require.NotNil(t, ephemeralAttr, "connection_string should have ephemeral attribute")
	val, diags := ephemeralAttr.Expr.Value(nil)
	require.False(t, diags.HasErrors())
	assert.True(t, val.True(), "ephemeral should be true")

	apiKeyVar := requireBlock(t, varsBody, "variable", "api_key")
	assert.NotNil(t, apiKeyVar)

	// Secret version variable should exist
	connectionStringVersionVar := requireBlock(t, varsBody, "variable", "connection_string_version")
	assert.NotNil(t, connectionStringVersionVar)
	assert.Equal(t, "number", expressionString(t, connectionStringVersionVar.Body.Attributes["type"].Expr))
	assert.Equal(t, "null", expressionString(t, connectionStringVersionVar.Body.Attributes["default"].Expr))
	
	// Version variable should have validation
	validationBlock := findBlock(connectionStringVersionVar.Body, "validation")
	require.NotNil(t, validationBlock, "version variable should have validation")
	conditionExpr := expressionString(t, validationBlock.Body.Attributes["condition"].Expr)
	assert.Contains(t, conditionExpr, "var.connection_string")
	assert.Contains(t, conditionExpr, "var.connection_string_version")

	// Check locals.tf - secret fields should NOT be in the body
	localsBody := parseHCLBody(t, "locals.tf")
	localsBlock := requireBlock(t, localsBody, "locals")
	localAttr := localsBlock.Body.Attributes["resource_body"]
	localExpr := expressionString(t, localAttr.Expr)
	
	// Normal field should be present (nested under properties)
	assert.Contains(t, localExpr, "normalField = var.properties.normal_field")
	
	// Secret fields should NOT be in locals
	assert.NotContains(t, localExpr, "connectionString")
	assert.NotContains(t, localExpr, "apiKey")

	// Check main.tf
	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	
	// Should have sensitive_body attribute
	sensitiveBodyAttr := resourceBlock.Body.Attributes["sensitive_body"]
	require.NotNil(t, sensitiveBodyAttr, "resource should have sensitive_body attribute")
	sensitiveBodyExpr := expressionString(t, sensitiveBodyAttr.Expr)
	assert.Contains(t, sensitiveBodyExpr, "var.connection_string")
	assert.Contains(t, sensitiveBodyExpr, "var.api_key")
	assert.Contains(t, sensitiveBodyExpr, "properties.connectionString")
	assert.Contains(t, sensitiveBodyExpr, "properties.apiKey")
	
	// Should have sensitive_body_version attribute
	sensitiveBodyVersionAttr := resourceBlock.Body.Attributes["sensitive_body_version"]
	require.NotNil(t, sensitiveBodyVersionAttr, "resource should have sensitive_body_version attribute")
	sensitiveBodyVersionExpr := expressionString(t, sensitiveBodyVersionAttr.Expr)
	assert.Contains(t, sensitiveBodyVersionExpr, "var.connection_string_version")
	assert.Contains(t, sensitiveBodyVersionExpr, "var.api_key_version")
}
