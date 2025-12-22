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
		{"balance-similar-node-groups", "balance_similar_node_groups"},
		{"foo.bar", "foo_bar"},
		{"foo bar", "foo_bar"},
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

	propertiesVar := findBlock(varsBody, "variable", "properties")
	assert.Nil(t, propertiesVar)

	writableVar := requireBlock(t, varsBody, "variable", "writable_prop")
	assert.Equal(t, "string", expressionString(t, writableVar.Body.Attributes["type"].Expr))
	assert.Equal(t, "null", expressionString(t, writableVar.Body.Attributes["default"].Expr))

	localsBody := parseHCLBody(t, "locals.tf")
	localsBlock := requireBlock(t, localsBody, "locals")
	localAttr := localsBlock.Body.Attributes["test_local"]
	localExpr := expressionString(t, localAttr.Expr)
	assert.Contains(t, localExpr, "location = var.location")
	assert.Contains(t, localExpr, "writableProp = var.writable_prop")
	assert.NotContains(t, localExpr, "readOnlyProp")
	assert.NotContains(t, localExpr, "var.properties")

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	assert.Equal(t, "testResource@2024-01-01", attributeStringValue(t, resourceBlock.Body.Attributes["type"]))
	assert.Equal(t, "var.name", expressionString(t, resourceBlock.Body.Attributes["name"].Expr))
	assert.Equal(t, "var.parent_id", expressionString(t, resourceBlock.Body.Attributes["parent_id"].Expr))
	assert.Equal(t, "var.location", expressionString(t, resourceBlock.Body.Attributes["location"].Expr))
	bodyExpr := expressionString(t, resourceBlock.Body.Attributes["body"].Expr)
	assert.Equal(t, "local.test_local", strings.TrimSpace(bodyExpr))
	assert.Nil(t, resourceBlock.Body.Attributes["tags"])

	outputsBody := parseHCLBody(t, "outputs.tf")
	idOutput := requireBlock(t, outputsBody, "output", "resource_id")
	assert.Equal(t, "The ID of the created resource.", attributeStringValue(t, idOutput.Body.Attributes["description"]))
	assert.Equal(t, "azapi_resource.this.id", expressionString(t, idOutput.Body.Attributes["value"].Expr))

	nameOutput := requireBlock(t, outputsBody, "output", "name")
	assert.Equal(t, "The name of the created resource.", attributeStringValue(t, nameOutput.Body.Attributes["description"]))
	assert.Equal(t, "azapi_resource.this.name", expressionString(t, nameOutput.Body.Attributes["value"].Expr))
}

func TestGenerate_QuotesNonIdentifierObjectKeysInLocals(t *testing.T) {
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
						"autoScalerProfile": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"balance-similar-node-groups": {
										Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
									},
									"foo.bar": {
										Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	apiVersion := "2025-10-01"
	err = Generate(schema, "Microsoft.ContainerService/managedClusters", "resource_body", apiVersion, false, false)
	require.NoError(t, err)

	localsBytes, err := os.ReadFile("locals.tf")
	require.NoError(t, err)
	locals := string(localsBytes)

	// Hyphenated keys don't need quotes in HCL.
	assert.Contains(t, locals, "balance-similar-node-groups")
	// RHS must reference a valid Terraform attribute name (snake_case).
	assert.Contains(t, locals, "var.auto_scaler_profile.balance_similar_node_groups")

	// Keys containing '.' must be quoted, otherwise HCL treats '.' as traversal syntax.
	assert.Contains(t, locals, "\"foo.bar\"")
	assert.Contains(t, locals, "var.auto_scaler_profile.foo_bar")
}

func TestGenerate_FailsOnFlattenedPropertiesNameCollision(t *testing.T) {
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
						// This would collide with the built-in top-level var "name".
						"name": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2025-01-01", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
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
	tokens := constructValue(schema, accessPath, false)

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
