package submodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
