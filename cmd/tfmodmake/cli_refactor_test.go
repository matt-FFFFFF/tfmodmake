package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddSubmodule tests that `add submodule` works correctly.
// This test does not require network access.
func TestAddSubmodule(t *testing.T) {
	// Create a minimal dummy submodule
	tmpDir := t.TempDir()
	submodulePath := filepath.Join(tmpDir, "modules", "testmod")
	if err := os.MkdirAll(submodulePath, 0o755); err != nil {
		t.Fatalf("Failed to create submodule dir: %v", err)
	}

	// Create minimal variables.tf
	variablesTf := `variable "parent_id" {
  type = string
}

variable "value" {
  type = string
}
`
	if err := os.WriteFile(filepath.Join(submodulePath, "variables.tf"), []byte(variablesTf), 0o644); err != nil {
		t.Fatalf("Failed to write variables.tf: %v", err)
	}

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Test add submodule command
	cmd := exec.Command(tfmodmakePath, "add", "submodule", "modules/testmod")
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run add submodule: %v\n%s", err, output)
	}

	// Verify wrapper files were created
	wrapperFiles := []string{"variables.testmod.tf", "main.testmod.tf"}
	for _, file := range wrapperFiles {
		path := filepath.Join(tmpDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s not created by add submodule", file)
		}
	}
}

// TestGenAVMDryRun tests that `tfmodmake gen avm --dry-run` produces no file changes.
// This test does not require network access because --dry-run exits before fetching data.
func TestGenAVMDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Run gen avm with --dry-run (resource is required but dry-run exits before network)
	cmd := exec.Command(tfmodmakePath, "gen", "avm", "-resource", "Microsoft.Test/parents", "-dry-run")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run gen avm --dry-run: %v\n%s", err, output)
	}

	// Verify output mentions dry run
	outputStr := string(output)
	if !strings.Contains(outputStr, "DRY RUN") {
		t.Errorf("Expected output to mention DRY RUN, got: %s", outputStr)
	}

	// Verify NO files were created
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read tmpDir: %v", err)
	}

	if len(entries) > 0 {
		for _, entry := range entries {
			t.Errorf("Unexpected file/directory created during dry run: %s", entry.Name())
		}
	}
}
