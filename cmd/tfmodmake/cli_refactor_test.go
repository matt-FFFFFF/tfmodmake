package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenAliasEquivalence tests that `tfmodmake gen` produces the same output as default generation
func TestGenAliasEquivalence(t *testing.T) {
	// Create a minimal test spec
	testSpec := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"version": "2024-01-01",
		},
		"paths": map[string]interface{}{
			"/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/testResources/{resourceName}": map[string]interface{}{
				"put": map[string]interface{}{
					"operationId": "TestResources_CreateOrUpdate",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "parameters",
							"in":       "body",
							"required": true,
							"schema": map[string]interface{}{
								"$ref": "#/definitions/TestResource",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"schema": map[string]interface{}{
								"$ref": "#/definitions/TestResource",
							},
						},
					},
				},
			},
		},
		"definitions": map[string]interface{}{
			"TestResource": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"properties": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	// Test default generation
	defaultDir := t.TempDir()
	specPath1 := filepath.Join(defaultDir, "test_spec.json")
	specData, err := json.MarshalIndent(testSpec, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test spec: %v", err)
	}
	if err := os.WriteFile(specPath1, specData, 0o644); err != nil {
		t.Fatalf("Failed to write test spec: %v", err)
	}

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Run default generation
	cmd := exec.Command(tfmodmakePath, "-spec", specPath1, "-resource", "Microsoft.Test/testResources")
	cmd.Dir = defaultDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run default generation: %v\n%s", err, output)
	}

	// Verify files were created
	defaultFiles := []string{"variables.tf", "locals.tf", "main.tf", "outputs.tf", "terraform.tf"}
	for _, file := range defaultFiles {
		path := filepath.Join(defaultDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s not created in default generation", file)
		}
	}

	// Verify main.interfaces.tf was NOT created
	interfacesPath := filepath.Join(defaultDir, "main.interfaces.tf")
	if _, err := os.Stat(interfacesPath); !os.IsNotExist(err) {
		t.Errorf("main.interfaces.tf should NOT be created by default generation")
	}

	// Test 'gen' subcommand
	genDir := t.TempDir()
	specPath2 := filepath.Join(genDir, "test_spec.json")
	if err := os.WriteFile(specPath2, specData, 0o644); err != nil {
		t.Fatalf("Failed to write test spec: %v", err)
	}

	cmd = exec.Command(tfmodmakePath, "gen", "-spec", specPath2, "-resource", "Microsoft.Test/testResources")
	cmd.Dir = genDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run gen subcommand: %v\n%s", err, output)
	}

	// Verify files were created
	for _, file := range defaultFiles {
		path := filepath.Join(genDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s not created in gen subcommand", file)
		}
	}

	// Verify main.interfaces.tf was NOT created
	interfacesPath = filepath.Join(genDir, "main.interfaces.tf")
	if _, err := os.Stat(interfacesPath); !os.IsNotExist(err) {
		t.Errorf("main.interfaces.tf should NOT be created by gen subcommand")
	}

	// Compare the core generated files (they should be identical)
	for _, file := range defaultFiles {
		defaultContent, err := os.ReadFile(filepath.Join(defaultDir, file))
		if err != nil {
			t.Fatalf("Failed to read %s from default dir: %v", file, err)
		}

		genContent, err := os.ReadFile(filepath.Join(genDir, file))
		if err != nil {
			t.Fatalf("Failed to read %s from gen dir: %v", file, err)
		}

		if string(defaultContent) != string(genContent) {
			t.Errorf("File %s differs between default and gen subcommand", file)
		}
	}
}

// TestAddAVMInterfaces tests that `tfmodmake add avm-interfaces` creates main.interfaces.tf
func TestAddAVMInterfaces(t *testing.T) {
	tmpDir := t.TempDir()

	// First generate a base module
	testSpec := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"version": "2024-01-01",
		},
		"paths": map[string]interface{}{
			"/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/testResources/{resourceName}": map[string]interface{}{
				"put": map[string]interface{}{
					"operationId": "TestResources_CreateOrUpdate",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "parameters",
							"in":       "body",
							"required": true,
							"schema": map[string]interface{}{
								"$ref": "#/definitions/TestResource",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"schema": map[string]interface{}{
								"$ref": "#/definitions/TestResource",
							},
						},
					},
				},
			},
		},
		"definitions": map[string]interface{}{
			"TestResource": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"properties": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	specPath := filepath.Join(tmpDir, "test_spec.json")
	specData, err := json.MarshalIndent(testSpec, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test spec: %v", err)
	}
	if err := os.WriteFile(specPath, specData, 0o644); err != nil {
		t.Fatalf("Failed to write test spec: %v", err)
	}

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Generate base module
	cmd := exec.Command(tfmodmakePath, "-spec", specPath, "-resource", "Microsoft.Test/testResources")
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate base module: %v\n%s", err, output)
	}

	// Verify main.interfaces.tf does NOT exist yet
	interfacesPath := filepath.Join(tmpDir, "main.interfaces.tf")
	if _, err := os.Stat(interfacesPath); !os.IsNotExist(err) {
		t.Fatalf("main.interfaces.tf should not exist before add avm-interfaces")
	}

	// Run add avm-interfaces
	cmd = exec.Command(tfmodmakePath, "add", "avm-interfaces", "-resource", "Microsoft.Test/testResources")
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run add avm-interfaces: %v\n%s", err, output)
	}

	// Verify main.interfaces.tf now exists
	if _, err := os.Stat(interfacesPath); os.IsNotExist(err) {
		t.Fatalf("main.interfaces.tf should exist after add avm-interfaces")
	}

	// Read and verify content
	content, err := os.ReadFile(interfacesPath)
	if err != nil {
		t.Fatalf("Failed to read main.interfaces.tf: %v", err)
	}

	contentStr := string(content)
	// Check for expected content
	if !strings.Contains(contentStr, "module \"avm_interfaces\"") {
		t.Errorf("main.interfaces.tf should contain module block")
	}
	if !strings.Contains(contentStr, "source") {
		t.Errorf("main.interfaces.tf should contain source attribute")
	}

	// Test idempotency - run again
	cmd = exec.Command(tfmodmakePath, "add", "avm-interfaces", "-resource", "Microsoft.Test/testResources")
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run add avm-interfaces second time (idempotency test): %v\n%s", err, output)
	}

	// Verify file still exists and has content
	if _, err := os.Stat(interfacesPath); os.IsNotExist(err) {
		t.Fatalf("main.interfaces.tf should still exist after second run")
	}
}

// TestAddSubmoduleAliasEquivalence tests that both `addsub` and `add submodule` work
func TestAddSubmoduleAliasEquivalence(t *testing.T) {
	// Create a minimal dummy submodule
	createDummySubmodule := func(dir string) {
		submodulePath := filepath.Join(dir, "modules", "testmod")
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
	}

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Test legacy addsub command
	addsubDir := t.TempDir()
	createDummySubmodule(addsubDir)

	cmd := exec.Command(tfmodmakePath, "addsub", "modules/testmod")
	cmd.Dir = addsubDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run addsub: %v\n%s", err, output)
	}

	// Verify wrapper files were created
	wrapperFiles := []string{"variables.testmod.tf", "main.testmod.tf"}
	for _, file := range wrapperFiles {
		path := filepath.Join(addsubDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s not created by addsub", file)
		}
	}

	// Test new add submodule command
	addSubmoduleDir := t.TempDir()
	createDummySubmodule(addSubmoduleDir)

	cmd = exec.Command(tfmodmakePath, "add", "submodule", "modules/testmod")
	cmd.Dir = addSubmoduleDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run add submodule: %v\n%s", err, output)
	}

	// Verify wrapper files were created
	for _, file := range wrapperFiles {
		path := filepath.Join(addSubmoduleDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s not created by add submodule", file)
		}
	}

	// Compare content (should be equivalent)
	for _, file := range wrapperFiles {
		addsubContent, err := os.ReadFile(filepath.Join(addsubDir, file))
		if err != nil {
			t.Fatalf("Failed to read %s from addsub dir: %v", file, err)
		}

		addSubmoduleContent, err := os.ReadFile(filepath.Join(addSubmoduleDir, file))
		if err != nil {
			t.Fatalf("Failed to read %s from add submodule dir: %v", file, err)
		}

		if string(addsubContent) != string(addSubmoduleContent) {
			t.Errorf("File %s differs between addsub and add submodule", file)
		}
	}
}

// TestDiscoverChildrenAliasEquivalence tests that both `children` and `discover children` work
func TestDiscoverChildrenAliasEquivalence(t *testing.T) {
	// This test requires actual spec files and network access, so we'll do a simpler
	// validation that the commands parse correctly and fail with expected error messages
	// when required args are missing

	// Build tfmodmake for testing
	tfmodmakePath := filepath.Join(t.TempDir(), "tfmodmake")
	buildCmd := exec.Command("go", "build", "-o", tfmodmakePath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build tfmodmake: %v\n%s", err, output)
	}

	// Test legacy children command fails with expected error when args missing
	cmd := exec.Command(tfmodmakePath, "children")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Expected children command to fail without required args")
	}
	outputStr := string(output)
	if !strings.Contains(outputStr, "parent") || !strings.Contains(outputStr, "required") {
		t.Logf("Expected error about missing parent arg, got: %s", outputStr)
	}

	// Test new discover children command fails with expected error when args missing
	cmd = exec.Command(tfmodmakePath, "discover", "children")
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Expected discover children command to fail without required args")
	}
	outputStr = string(output)
	if !strings.Contains(outputStr, "parent") || !strings.Contains(outputStr, "required") {
		t.Logf("Expected error about missing parent arg, got: %s", outputStr)
	}
}
