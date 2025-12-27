package submodule

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

// Generate reads a Terraform submodule at modulePath and writes variables.submodule.tf and main.submodule.tf
// in the current working directory to expose the submodule as a map-based module block.
func Generate(modulePath string) error {
	cleanPath := filepath.Clean(modulePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to stat module path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("module path is not a directory: %s", cleanPath)
	}

	module, diags := tfconfig.LoadModule(cleanPath)
	if diags.HasErrors() {
		return diags.Err()
	}

	moduleName := sanitizeName(filepath.Base(cleanPath))
	if moduleName == "" {
		moduleName = "module"
	}

	typeTokens, err := buildTypeTokens(module)
	if err != nil {
		return fmt.Errorf("failed to build variable type: %w", err)
	}

	desc := buildDescription(module)

	if err := writeVariablesFile(moduleName, typeTokens, desc); err != nil {
		return fmt.Errorf("failed to write variables.submodule.tf: %w", err)
	}

	if err := writeMainFile(moduleName, cleanPath, module); err != nil {
		return fmt.Errorf("failed to write main.submodule.tf: %w", err)
	}

	return nil
}

func buildDescription(module *tfconfig.Module) string {
	sb := strings.Builder{}
	sb.WriteString("Map of instances for the submodule with the following attributes:\n\n")
	for k, v := range module.Variables {
		if k == "parent_id" {
			continue
		}
		sb.WriteString(fmt.Sprintf("**%s**\n%s\n", k, v.Description))
	}
	return sb.String()
}

func buildTypeTokens(module *tfconfig.Module) (hclwrite.Tokens, error) {
	var variableNames []string
	for name := range module.Variables {
		if name == "parent_id" {
			continue
		}
		variableNames = append(variableNames, name)
	}
	sort.Strings(variableNames)

	var attrs []hclwrite.ObjectAttrTokens
	for _, name := range variableNames {
		variable := module.Variables[name]
		typeExpr := strings.TrimSpace(variable.Type)
		if typeExpr == "" {
			typeExpr = "any"
		}

		typeTokens, err := parseExpressionTokens(typeExpr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse type expression for %s: %w", name, err)
		}

		if !variable.Required {
			typeTokens = hclwrite.TokensForFunctionCall("optional", typeTokens)
		}

		attrs = append(attrs, hclwrite.ObjectAttrTokens{
			Name:  hclwrite.TokensForIdentifier(name),
			Value: typeTokens,
		})
	}

	return hclwrite.TokensForFunctionCall("map", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject(attrs))), nil
}

func writeVariablesFile(moduleName string, typeTokens hclwrite.Tokens, description string) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	block := body.AppendNewBlock("variable", []string{moduleName})
	blockBody := block.Body()
	blockBody.SetAttributeRaw("description", hclgen.TokensForHeredoc(strings.TrimSpace(description)))
	blockBody.SetAttributeRaw("type", typeTokens)
	blockBody.SetAttributeValue("default", cty.MapValEmpty(cty.DynamicPseudoType))

	filename := fmt.Sprintf("variables.%s.tf", moduleName)
	return os.WriteFile(filename, file.Bytes(), 0o644)
}

func writeMainFile(moduleName, sourcePath string, module *tfconfig.Module) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	block := body.AppendNewBlock("module", []string{moduleName})
	blockBody := block.Body()
	blockBody.SetAttributeValue("source", cty.StringVal(fmt.Sprintf("./%s", sourcePath)))

	blockBody.SetAttributeRaw("for_each", hclgen.TokensForTraversal("var", moduleName))

	var variableNames []string
	for name := range module.Variables {
		variableNames = append(variableNames, name)
	}
	sort.Strings(variableNames)

	for _, name := range variableNames {
		if name == "parent_id" {
			blockBody.SetAttributeRaw(name, hclgen.TokensForTraversal("azapi_resource", "this", "id"))
			continue
		}

		blockBody.SetAttributeRaw(name, hclgen.TokensForTraversal("each", "value", name))
	}

	filename := fmt.Sprintf("main.%s.tf", moduleName)
	return os.WriteFile(filename, file.Bytes(), 0o644)
}

func parseExpressionTokens(expr string) (hclwrite.Tokens, error) {
	snippet := fmt.Sprintf("value = %s\n", expr)
	file, diags := hclwrite.ParseConfig([]byte(snippet), "expression.hcl", hcl.Pos{})
	if diags.HasErrors() {
		return nil, diags
	}
	attr := file.Body().GetAttribute("value")
	if attr == nil {
		return nil, fmt.Errorf("failed to parse expression")
	}
	return attr.Expr().BuildTokens(nil), nil
}

func sanitizeName(name string) string {
	lowered := strings.ToLower(name)
	replacer := regexp.MustCompile(`[^a-z0-9_]+`)
	sanitized := replacer.ReplaceAllString(lowered, "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return ""
	}
	if sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "mod_" + sanitized
	}
	sanitized = strings.ReplaceAll(sanitized, "__", "_")
	return sanitized
}
