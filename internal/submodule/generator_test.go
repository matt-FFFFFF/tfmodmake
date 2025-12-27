package submodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

func TestGenerateCreatesSubmoduleFiles(t *testing.T) {
	tempDir := t.TempDir()
	moduleDir := filepath.Join(tempDir, "my-module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}

	variableHCL := `
variable "region" {
  type = string
}

variable "replicas" {
  type    = number
  default = 1
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(variableHCL), 0o644); err != nil {
		t.Fatalf("failed to write module variables: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	if err := Generate("my-module"); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	varsContent, err := os.ReadFile(filepath.Join(tempDir, "variables.my_module.tf"))
	if err != nil {
		t.Fatalf("failed to read variables.my_module.tf: %v", err)
	}
	if !strings.Contains(string(varsContent), `variable "my_module"`) {
		t.Fatalf("variables file missing variable block")
	}
	if !strings.Contains(string(varsContent), "region   = string") {
		t.Fatalf("variables file missing region type")
	}
	if !strings.Contains(string(varsContent), "replicas = optional(number)") {
		t.Fatalf("variables file missing optional replicas")
	}

	mainContent, err := os.ReadFile(filepath.Join(tempDir, "main.my_module.tf"))
	if err != nil {
		t.Fatalf("failed to read main.my_module.tf: %v", err)
	}
	if !strings.Contains(string(mainContent), `module "my_module"`) {
		t.Fatalf("main file missing module block")
	}
	if !strings.Contains(string(mainContent), "for_each = var.my_module") {
		t.Fatalf("main file missing for_each")
	}
	if !strings.Contains(string(mainContent), "replicas = each.value.replicas") {
		t.Fatalf("main file missing replicas argument")
	}
}

func TestBuildTypeTokensMarksNonRequiredAsOptional(t *testing.T) {
	module := &tfconfig.Module{
		Variables: map[string]*tfconfig.Variable{
			"subject": {
				Type:     "string",
				Required: false,
			},
			"claims_matching_expression": {
				Type:     "string",
				Required: true,
			},
		},
	}

	tokens, err := buildTypeTokens(module)
	if err != nil {
		t.Fatalf("buildTypeTokens returned error: %v", err)
	}

	content := string(tokens.Bytes())
	// hclwrite aligns attributes, so we need to be flexible with whitespace
	if !strings.Contains(content, "subject") || !strings.Contains(content, "optional(string)") {
		t.Fatalf("expected subject to be optional, got: %s", content)
	}
	// Check that subject is assigned optional(string)
	lines := strings.Split(content, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "subject") && strings.Contains(line, "optional(string)") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected subject line to contain optional(string), got: %s", content)
	}

	if strings.Contains(content, "claims_matching_expression = optional(string)") {
		t.Fatalf("claims_matching_expression should not be optional, got: %s", content)
	}
}
