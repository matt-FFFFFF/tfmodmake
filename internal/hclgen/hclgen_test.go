package hclgen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokensForHeredoc(t *testing.T) {
	desc := "This is a description."
	tokens := TokensForHeredoc(desc)
	output := string(tokens.Bytes())

	expected := "<<DESCRIPTION\nThis is a description.\nDESCRIPTION"
	assert.Equal(t, expected, output)
}

func TestTokensForPath(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "single part",
			parts:    []string{"var"},
			expected: "var",
		},
		{
			name:     "two parts",
			parts:    []string{"var", "name"},
			expected: "var.name",
		},
		{
			name:     "three parts",
			parts:    []string{"azapi_resource", "this", "id"},
			expected: "azapi_resource.this.id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := TokensForTraversal(tt.parts...)
			assert.Equal(t, tt.expected, string(tokens.Bytes()))
		})
	}
}

func TestTokensForTraversalOrIndex(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "simple identifiers use dot traversal",
			parts:    []string{"azapi_resource", "this", "output", "properties", "foo"},
			expected: "azapi_resource.this.output.properties.foo",
		},
		{
			name:     "hyphenated key uses bracket traversal",
			parts:    []string{"azapi_resource", "this", "output", "properties", "foo-bar"},
			expected: "azapi_resource.this.output.properties[\"foo-bar\"]",
		},
		{
			name:     "mixed path uses both dot and bracket traversal",
			parts:    []string{"azapi_resource", "this", "output", "properties", "foo-bar", "baz"},
			expected: "azapi_resource.this.output.properties[\"foo-bar\"].baz",
		},
		{
			name:     "keys with spaces use bracket traversal",
			parts:    []string{"azapi_resource", "this", "output", "properties", "foo bar"},
			expected: "azapi_resource.this.output.properties[\"foo bar\"]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := TokensForTraversalOrIndex(tt.parts...)
			assert.Equal(t, tt.expected, string(tokens.Bytes()))
		})
	}
}

func TestTokensForMultilineStringList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		tokens := TokensForMultilineStringList(nil)
		assert.Equal(t, "[]", string(tokens.Bytes()))
	})

	t.Run("single element", func(t *testing.T) {
		tokens := TokensForMultilineStringList([]string{"a"})
		assert.Equal(t, "[\n\"a\"\n]", string(tokens.Bytes()))
	})

	t.Run("multiple elements", func(t *testing.T) {
		tokens := TokensForMultilineStringList([]string{"a", "b"})
		assert.Equal(t, "[\n\"a\",\n\"b\"\n]", string(tokens.Bytes()))
	})
}

func TestTernary(t *testing.T) {
	condition := hclwrite.TokensForIdentifier("var.enabled")
	trueExpr := hclwrite.TokensForIdentifier("var.value")

	tokens := NullEqualityTernary(condition, trueExpr)
	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("attr", tokens)
	expected := "attr = var.enabled == null ? null : var.value\n"
	buf := new(bytes.Buffer)
	_, err := f.WriteTo(buf)
	require.NoError(t, err)
	parsed, diags := hclwrite.ParseConfig(buf.Bytes(), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	attr := parsed.Body().GetAttribute("attr")
	resultTokens := attr.BuildTokens(nil)
	assert.Equal(t, expected, string(resultTokens.Bytes()))
}

func TestSetDescriptionAttribute(t *testing.T) {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	desc := "My description"
	SetDescriptionAttribute(body, desc)

	output := string(f.Bytes())
	expected := `description = <<DESCRIPTION
My description
DESCRIPTION
`
	assert.Equal(t, expected, output)
}

func TestWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.tf")

	f := hclwrite.NewEmptyFile()
	body := f.Body()
	body.SetAttributeRaw("foo", hclwrite.TokensForIdentifier("bar"))

	err := WriteFile(filePath, f)
	require.NoError(t, err)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "foo = bar")
}
