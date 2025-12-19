package terraform

import (
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

	apiVersion := "2024-01-01"
	err = Generate(schema, "testResource", "test_local", apiVersion, supportsTags)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")

	nameVar := requireBlock(t, varsBody, "variable", "name")
	assert.Equal(t, "The name of the resource.", attributeStringValue(t, nameVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, nameVar.Body.Attributes["type"].Expr))

	parentVar := requireBlock(t, varsBody, "variable", "parent_id")
	assert.Equal(t, "The parent resource ID for this resource.", attributeStringValue(t, parentVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, parentVar.Body.Attributes["type"].Expr))

	assert.Nil(t, findBlock(varsBody, "variable", "tags"))

	locationVar := requireBlock(t, varsBody, "variable", "location")
	assert.Equal(t, "Resource location", attributeStringValue(t, locationVar.Body.Attributes["description"]))
	assert.Equal(t, "string", expressionString(t, locationVar.Body.Attributes["type"].Expr))
	assert.Equal(t, "null", expressionString(t, locationVar.Body.Attributes["default"].Expr))

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

	err = Generate(schema, "testResource", "local_map", "", false)
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

	err = Generate(schema, "testResource", "resource_body", "", true)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	tagsVar := requireBlock(t, varsBody, "variable", "tags")
	assert.Equal(t, "map(string)", expressionString(t, tagsVar.Body.Attributes["type"].Expr))
	assert.Equal(t, "null", expressionString(t, tagsVar.Body.Attributes["default"].Expr))

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")
	assert.Equal(t, "var.tags", expressionString(t, resourceBlock.Body.Attributes["tags"].Expr))
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
	err = Generate(schema, "testResource", "placeholder_local", "", supportsTags)
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

	err = Generate(nil, "testResource", "unused_local", "2024-01-01", false)
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
			want: "object({\n    prop1 = optional(string)\n  })",
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
			want: "map(object({\n    max_concurrent = optional(number)\n    query_logging = string\n  }))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapType(tt.schema)
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

	got := constructValue(schema, "var.kube_dns_overrides", false)

	expected := "var.kube_dns_overrides == null ? null : { for k, value in var.kube_dns_overrides : k => value == null ? null : {\nmaxConcurrent = value.max_concurrent\nqueryLogging = value.query_logging\n} }"
	assert.Equal(t, expected, got)
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
