package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ParseHCLFile reads and parses an HCL file from disk using hclwrite.
func ParseHCLFile(path string) (*hclwrite.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	file, diags := hclwrite.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing %s: %s", path, diags.Error())
	}
	return file, nil
}

// ParseModuleFile is a convenience wrapper that parses a named HCL file inside a module directory.
func ParseModuleFile(moduleDir, filename string) (*hclwrite.File, error) {
	return ParseHCLFile(filepath.Join(moduleDir, filename))
}

// ExtractResourceTypeAndVersion parses a main.tf hclwrite.File and returns the
// resource type and API version from the azapi_resource "this" type attribute.
//
// Input type value: "Microsoft.App/managedEnvironments@2025-10-02-preview"
// Returns: ("Microsoft.App/managedEnvironments", "2025-10-02-preview", nil)
func ExtractResourceTypeAndVersion(mainFile *hclwrite.File) (string, string, error) {
	if mainFile == nil {
		return "", "", fmt.Errorf("main file is nil")
	}

	for _, block := range mainFile.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "azapi_resource" || labels[1] != "this" {
			continue
		}
		typeAttr := block.Body().GetAttribute("type")
		if typeAttr == nil {
			return "", "", fmt.Errorf("azapi_resource \"this\" has no type attribute")
		}
		typeVal := extractQuotedStringFromTokens(typeAttr.Expr().BuildTokens(nil))
		if typeVal == "" {
			return "", "", fmt.Errorf("could not extract type string from azapi_resource \"this\"")
		}
		return splitTypeAndVersion(typeVal)
	}

	return "", "", fmt.Errorf("no azapi_resource \"this\" block found in main.tf")
}

// splitTypeAndVersion splits "Microsoft.App/managedEnvironments@2025-10-02-preview"
// into ("Microsoft.App/managedEnvironments", "2025-10-02-preview").
func splitTypeAndVersion(typeStr string) (string, string, error) {
	idx := strings.LastIndex(typeStr, "@")
	if idx < 0 {
		return "", "", fmt.Errorf("type string %q does not contain @apiVersion", typeStr)
	}
	resourceType := typeStr[:idx]
	apiVersion := typeStr[idx+1:]
	if resourceType == "" || apiVersion == "" {
		return "", "", fmt.Errorf("invalid type string %q: empty resource type or API version", typeStr)
	}
	return resourceType, apiVersion, nil
}

// extractQuotedStringFromTokens extracts the first quoted string literal value
// from a sequence of HCL tokens.
func extractQuotedStringFromTokens(tokens hclwrite.Tokens) string {
	var parts []string
	inQuote := false
	for _, tok := range tokens {
		switch {
		case tok.Type == hclsyntax.TokenOQuote:
			inQuote = true
		case tok.Type == hclsyntax.TokenCQuote:
			inQuote = false
		case inQuote:
			parts = append(parts, string(tok.Bytes))
		}
	}
	return strings.Join(parts, "")
}

// ExtractVariableTypes returns a map of variable name to type expression tokens
// from a parsed variables.tf file.
func ExtractVariableTypes(varsFile *hclwrite.File) map[string]hclwrite.Tokens {
	if varsFile == nil {
		return nil
	}
	result := make(map[string]hclwrite.Tokens)
	for _, block := range varsFile.Body().Blocks() {
		if block.Type() != "variable" {
			continue
		}
		labels := block.Labels()
		if len(labels) == 0 {
			continue
		}
		varName := labels[0]
		typeAttr := block.Body().GetAttribute("type")
		if typeAttr == nil {
			continue
		}
		result[varName] = typeAttr.Expr().BuildTokens(nil)
	}
	return result
}

// ExtractVariableBlocks returns a map of variable name to the full variable block
// from a parsed variables.tf file.
func ExtractVariableBlocks(varsFile *hclwrite.File) map[string]*hclwrite.Block {
	if varsFile == nil {
		return nil
	}
	result := make(map[string]*hclwrite.Block)
	for _, block := range varsFile.Body().Blocks() {
		if block.Type() != "variable" {
			continue
		}
		labels := block.Labels()
		if len(labels) == 0 {
			continue
		}
		result[labels[0]] = block
	}
	return result
}

// ExtractLocalAssignments returns a map of local name to expression tokens
// from a parsed locals.tf file.
func ExtractLocalAssignments(localsFile *hclwrite.File) map[string]hclwrite.Tokens {
	if localsFile == nil {
		return nil
	}
	result := make(map[string]hclwrite.Tokens)
	for _, block := range localsFile.Body().Blocks() {
		if block.Type() != "locals" {
			continue
		}
		for name, attr := range block.Body().Attributes() {
			result[name] = attr.Expr().BuildTokens(nil)
		}
	}
	return result
}
