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
	err = Generate(schema, "testResource", "test_local", apiVersion, supportsTags, supportsLocation, nil)
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

	readOnlyOutput := requireBlock(t, outputsBody, "output", "read_only_prop")
	assert.Equal(t, "Computed value exported from the Azure API response.", attributeStringValue(t, readOnlyOutput.Body.Attributes["description"]))
	assert.Equal(t, "try(azapi_resource.this.output.properties.readOnlyProp, null)", expressionString(t, readOnlyOutput.Body.Attributes["value"].Expr))
}

func TestGenerate_NameVariable_UsesNameSchemaValidations(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	maxLen := uint64(63)
	nameSchema := &openapi3.Schema{
		Type:      &openapi3.Types{"string"},
		MinLength: 1,
		MaxLength: &maxLen,
		Pattern:   "^[a-z0-9-]{1,63}$",
	}

	err = Generate(nil, "testResource", "unused_local", "2024-01-01", false, false, nameSchema)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	nameVar := requireBlock(t, varsBody, "variable", "name")

	var validations []*hclsyntax.Block
	for _, b := range nameVar.Body.Blocks {
		if b.Type == "validation" {
			validations = append(validations, b)
		}
	}
	require.GreaterOrEqual(t, len(validations), 3)

	var conditions []string
	for _, b := range validations {
		condAttr := b.Body.Attributes["condition"]
		require.NotNil(t, condAttr)
		conditions = append(conditions, expressionString(t, condAttr.Expr))
	}
	joined := strings.Join(conditions, "\n")

	assert.Contains(t, joined, "length(var.name) >= 1")
	assert.Contains(t, joined, "length(var.name) <= 63")
	assert.Contains(t, joined, "can(regex(\"^[a-z0-9-]{1,63}$\", var.name))")
}

func TestGenerate_NestedObjectValidations(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	maxUsernameLength := uint64(20)
	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"windowsProfile": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"object"},
								Required: []string{"adminUsername"},
								Properties: map[string]*openapi3.SchemaRef{
									"adminUsername": {
										Value: &openapi3.Schema{
											Type:      &openapi3.Types{"string"},
											MinLength: 1,
											MaxLength: &maxUsernameLength,
										},
									},
									"licenseType": {
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"string"},
											Enum: []any{"None", "Windows_Server"},
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

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)

	err = Generate(schema, "Microsoft.Test/testResource", "resource_body", "2024-01-01", supportsTags, supportsLocation, nil)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")
	winVar := requireBlock(t, varsBody, "variable", "windows_profile")

	var validations []*hclsyntax.Block
	for _, b := range winVar.Body.Blocks {
		if b.Type == "validation" {
			validations = append(validations, b)
		}
	}
	require.GreaterOrEqual(t, len(validations), 2)

	var conditions []string
	for _, b := range validations {
		condAttr := b.Body.Attributes["condition"]
		require.NotNil(t, condAttr)
		conditions = append(conditions, expressionString(t, condAttr.Expr))
	}
	joined := strings.Join(conditions, "\n")

	assert.Contains(t, joined, "var.windows_profile == null")
	assert.Contains(t, joined, "var.windows_profile.admin_username")
	assert.Contains(t, joined, "length(var.windows_profile.admin_username) <= 20")
	assert.Contains(t, joined, "var.windows_profile.license_type")
	assert.Contains(t, joined, "contains(")
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
	err = Generate(schema, "Microsoft.ContainerService/managedClusters", "resource_body", apiVersion, false, false, nil)
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

func TestGenerate_SkipsSecretsByFullPathNotLeafName(t *testing.T) {
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
						"a": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"password": {
										Value: &openapi3.Schema{
											Type:       &openapi3.Types{"string"},
											Extensions: map[string]any{"x-ms-secret": true},
										},
									},
								},
							},
						},
						"b": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"password": {
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

	err = Generate(schema, "testResource", "resource_body", "2025-01-01", false, false, nil)
	require.NoError(t, err)

	localsBytes, err := os.ReadFile("locals.tf")
	require.NoError(t, err)
	locals := string(localsBytes)

	// Only the specific secret path should be removed.
	assert.NotContains(t, locals, "var.a.password")
	assert.Contains(t, locals, "var.b.password")
}

func TestGenerate_MainEnablesIgnoreNullProperty(t *testing.T) {
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
						"optionalString": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
						"optionalObj": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"nestedOptional": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
								},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2025-01-01", false, false, nil)
	require.NoError(t, err)

	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	main := string(mainBytes)

	assert.Contains(t, main, "ignore_null_property")
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

	err = Generate(schema, "testResource", "resource_body", "2025-01-01", false, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
}

func TestGenerate_DoesNotDuplicateSecretVarsFromFlattenedProperties(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	secretSchema := &openapi3.Schema{
		Type:        &openapi3.Types{"string"},
		Description: "A secret string",
		Extensions: map[string]any{
			"x-ms-secret": true,
		},
	}

	schema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"properties": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"daprAIConnectionString": {Value: secretSchema},
						"writableProp":           {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2025-01-01", false, false, nil)
	require.NoError(t, err)

	varsBody := parseHCLBody(t, "variables.tf")

	countVar := func(name string) int {
		count := 0
		for _, b := range varsBody.Blocks {
			if b.Type != "variable" {
				continue
			}
			if len(b.Labels) == 1 && b.Labels[0] == name {
				count++
			}
		}
		return count
	}

	// The secret is already surfaced as a flattened variable; it must not be redeclared.
	assert.Equal(t, 1, countVar("dapr_ai_connection_string"))

	secretVar := requireBlock(t, varsBody, "variable", "dapr_ai_connection_string")
	assert.Equal(t, "true", expressionString(t, secretVar.Body.Attributes["ephemeral"].Expr))

	// Version tracker variable should be present exactly once.
	assert.Equal(t, 1, countVar("dapr_ai_connection_string_version"))

	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	assert.Contains(t, string(mainBytes), "daprAIConnectionString")
	assert.Contains(t, string(mainBytes), "var.dapr_ai_connection_string")
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

	err = Generate(schema, "testResource", "local_map", "", false, supportsLocation, nil)
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

	err = Generate(schema, "testResource", "resource_body", "", true, supportsLocation, nil)
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

	err = Generate(schema, "testResource", "placeholder_local", "", supportsTags, supportsLocation, nil)
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

	err = Generate(nil, "testResource", "unused_local", "2024-01-01", false, false, nil)
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
	tokens := constructValue(schema, accessPath, false, nil, "", false)

	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("attr", tokens)
	buf := new(bytes.Buffer)
	_, err := f.WriteTo(buf)
	require.NoError(t, err)
	parsed, diags := hclwrite.ParseConfig(buf.Bytes(), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	attr := parsed.Body().GetAttribute("attr")
	resultTokens := attr.BuildTokens(nil)
	expected := `attr = var.kube_dns_overrides == null ? null : { for k, value in var.kube_dns_overrides : k => value == null ? null : {
  maxConcurrent = value.max_concurrent
  queryLogging  = value.query_logging
} }
`
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

	err = Generate(schema, "Microsoft.Test/testResource", "resource_body", "2024-01-01", false, false, nil)
	require.NoError(t, err)

	// Check variables.tf
	varsBody := parseHCLBody(t, "variables.tf")

	// On the merged branch, properties are flattened into individual variables
	// Normal field should be generated as a variable
	normalFieldVar := requireBlock(t, varsBody, "variable", "normal_field")
	assert.NotNil(t, normalFieldVar)
	assert.Equal(t, "string", expressionString(t, normalFieldVar.Body.Attributes["type"].Expr))

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

	// With flattened properties, normal field should be present at properties.normalField = var.normal_field
	assert.Contains(t, localExpr, "normalField = var.normal_field")

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
	assert.Contains(t, sensitiveBodyExpr, "properties")
	assert.Contains(t, sensitiveBodyExpr, "connectionString")
	assert.Contains(t, sensitiveBodyExpr, "apiKey")

	// Should have sensitive_body_version attribute
	sensitiveBodyVersionAttr := resourceBlock.Body.Attributes["sensitive_body_version"]
	require.NotNil(t, sensitiveBodyVersionAttr, "resource should have sensitive_body_version attribute")
	sensitiveBodyVersionExpr := expressionString(t, sensitiveBodyVersionAttr.Expr)
	assert.Contains(t, sensitiveBodyVersionExpr, "var.connection_string_version")
	assert.Contains(t, sensitiveBodyVersionExpr, "var.api_key_version")
}

func TestGenerate_ResponseExportValues(t *testing.T) {
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
						"defaultDomain": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								ReadOnly:    true,
								Description: "Default domain",
							},
						},
						"staticIp": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								ReadOnly:    true,
								Description: "Static IP",
							},
						},
						"provisioningState": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								ReadOnly:    true,
								Description: "Provisioning state",
							},
						},
						"writableField": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "Writable field",
							},
						},
					},
				},
			},
			"identity": {
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"principalId": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								ReadOnly:    true,
								Description: "Principal ID",
							},
						},
						"tenantId": {
							Value: &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								ReadOnly:    true,
								Description: "Tenant ID",
							},
						},
					},
				},
			},
		},
	}

	apiVersion := "2024-01-01"
	err = Generate(schema, "Microsoft.App/managedEnvironments", "resource_body", apiVersion, false, true, nil)
	require.NoError(t, err)

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")

	// Check that response_export_values is populated
	responseExportAttr := resourceBlock.Body.Attributes["response_export_values"]
	require.NotNil(t, responseExportAttr, "response_export_values attribute should exist")

	exprStr := expressionString(t, responseExportAttr.Expr)

	// Should contain the readOnly fields
	assert.Contains(t, exprStr, "properties.defaultDomain")
	assert.Contains(t, exprStr, "properties.staticIp")
	assert.Contains(t, exprStr, "properties.provisioningState")
	assert.Contains(t, exprStr, "identity.principalId")
	assert.Contains(t, exprStr, "identity.tenantId")

	// Should NOT contain writable fields
	assert.NotContains(t, exprStr, "writableField")
	assert.NotContains(t, exprStr, "location")

	// Read the full main.tf to check for the comment
	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	mainContent := string(mainBytes)

	// Check for the comment about trimming
	assert.Contains(t, mainContent, "Trim response_export_values")
}

func TestGenerate_ResponseExportValuesWithBlocklist(t *testing.T) {
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
						"goodField": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"string"},
								ReadOnly: true,
							},
						},
						"eTag": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"string"},
								ReadOnly: true,
							},
						},
						"createdAt": {
							Value: &openapi3.Schema{
								Type:     &openapi3.Types{"string"},
								ReadOnly: true,
							},
						},
						"status": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"phase": {
										Value: &openapi3.Schema{
											Type:     &openapi3.Types{"string"},
											ReadOnly: true,
										},
									},
								},
							},
						},
						"provisioningError": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"code": {
										Value: &openapi3.Schema{
											Type:     &openapi3.Types{"string"},
											ReadOnly: true,
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

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false, nil)
	require.NoError(t, err)

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")

	responseExportAttr := resourceBlock.Body.Attributes["response_export_values"]
	require.NotNil(t, responseExportAttr)

	exprStr := expressionString(t, responseExportAttr.Expr)

	// Should contain the good field
	assert.Contains(t, exprStr, "properties.goodField")

	// Should NOT contain blocklisted fields
	assert.NotContains(t, exprStr, "eTag")
	assert.NotContains(t, exprStr, "createdAt")
	assert.NotContains(t, exprStr, "status.phase")
	assert.NotContains(t, exprStr, "provisioningError")
}

func TestGenerate_ResponseExportValuesEmptyWhenNoReadOnly(t *testing.T) {
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
						"writableField": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
							},
						},
					},
				},
			},
		},
	}

	err = Generate(schema, "testResource", "resource_body", "2024-01-01", false, false, nil)
	require.NoError(t, err)

	mainBody := parseHCLBody(t, "main.tf")
	resourceBlock := requireBlock(t, mainBody, "resource", "azapi_resource", "this")

	responseExportAttr := resourceBlock.Body.Attributes["response_export_values"]
	require.NotNil(t, responseExportAttr)

	exprStr := strings.TrimSpace(expressionString(t, responseExportAttr.Expr))

	// Should be an empty list
	assert.Equal(t, "[]", exprStr)

	// Should NOT have the comment about trimming
	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	mainContent := string(mainBytes)
	assert.NotContains(t, mainContent, "Trim response_export_values")
}
