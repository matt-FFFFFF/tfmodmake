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

	if err := writeVariablesFile(moduleName, typeTokens); err != nil {
		return fmt.Errorf("failed to write variables.submodule.tf: %w", err)
	}

	if err := writeMainFile(moduleName, cleanPath, module); err != nil {
		return fmt.Errorf("failed to write main.submodule.tf: %w", err)
	}

	return nil
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

	var sb strings.Builder
	sb.WriteString("map(object({\n")
	for _, name := range variableNames {
		variable := module.Variables[name]
		typeExpr := strings.TrimSpace(variable.Type)
		if typeExpr == "" {
			typeExpr = "any"
		}
		if variable.Default != nil {
			typeExpr = fmt.Sprintf("optional(%s)", typeExpr)
		}
		sb.WriteString("  ")
		sb.WriteString(name)
		sb.WriteString(" = ")
		sb.WriteString(typeExpr)
		sb.WriteString("\n")
	}
	sb.WriteString("}))")

	return parseExpressionTokens(sb.String())
}

func writeVariablesFile(moduleName string, typeTokens hclwrite.Tokens) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	block := body.AppendNewBlock("variable", []string{moduleName})
	blockBody := block.Body()
	blockBody.SetAttributeValue("description", cty.StringVal(fmt.Sprintf("Instances of the %s submodule.", moduleName)))
	blockBody.SetAttributeRaw("type", typeTokens)

	filename := fmt.Sprintf("variables.%s.tf", moduleName)
	return os.WriteFile(filename, file.Bytes(), 0o644)
}

func writeMainFile(moduleName, sourcePath string, module *tfconfig.Module) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	block := body.AppendNewBlock("module", []string{moduleName})
	blockBody := block.Body()
	blockBody.SetAttributeValue("source", cty.StringVal(sourcePath))

	forEachTokens, err := parseExpressionTokens(fmt.Sprintf("var.%s", moduleName))
	if err != nil {
		return fmt.Errorf("failed to build for_each expression: %w", err)
	}
	blockBody.SetAttributeRaw("for_each", forEachTokens)

	var variableNames []string
	for name := range module.Variables {
		variableNames = append(variableNames, name)
	}
	sort.Strings(variableNames)

	for _, name := range variableNames {
		if name == "parent_id" {
			exprTokens, err := parseExpressionTokens("azapi_resource.this.id")
			if err != nil {
				return fmt.Errorf("failed to build parent_id expression: %w", err)
			}
			blockBody.SetAttributeRaw(name, exprTokens)
			continue
		}

		exprTokens, err := parseExpressionTokens(fmt.Sprintf("each.value.%s", name))
		if err != nil {
			return fmt.Errorf("failed to build expression for %s: %w", name, err)
		}
		blockBody.SetAttributeRaw(name, exprTokens)
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
