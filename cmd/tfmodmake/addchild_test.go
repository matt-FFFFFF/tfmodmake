package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveModuleName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple child resource",
			input:    "Microsoft.App/managedEnvironments/storages",
			expected: "storages",
		},
		{
			name:     "child resource with placeholder",
			input:    "Microsoft.App/managedEnvironments/storages/{storageName}",
			expected: "storages",
		},
		{
			name:     "KeyVault secrets",
			input:    "Microsoft.KeyVault/vaults/secrets",
			expected: "secrets",
		},
		{
			name:     "certificates",
			input:    "Microsoft.App/managedEnvironments/certificates",
			expected: "certificates",
		},
		{
			name:     "single segment",
			input:    "storages",
			expected: "storages",
		},
		{
			name:     "deeply nested",
			input:    "Microsoft.Foo/bars/bazs/quxs",
			expected: "quxs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveModuleName(tt.input)
			if result != tt.expected {
				t.Errorf("deriveModuleName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateChildModuleIdempotency(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a minimal test spec
	testSpec := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"version": "2024-01-01",
		},
		"paths": map[string]interface{}{
			"/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/parents/{parentName}/children/{childName}": map[string]interface{}{
				"put": map[string]interface{}{
					"operationId": "Children_CreateOrUpdate",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "parameters",
							"in":       "body",
							"required": true,
							"schema": map[string]interface{}{
								"$ref": "#/definitions/Child",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"schema": map[string]interface{}{
								"$ref": "#/definitions/Child",
							},
						},
					},
				},
			},
		},
		"definitions": map[string]interface{}{
			"Child": map[string]interface{}{
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

	modulePath := filepath.Join(tmpDir, "modules", "children")

	// First generation
	if err := generateChildModule([]string{specPath}, "Microsoft.Test/parents/children", modulePath); err != nil {
		t.Fatalf("First generation failed: %v", err)
	}

	// Read generated files
	files := []string{"variables.tf", "locals.tf", "main.tf", "outputs.tf", "terraform.tf"}
	firstGenContent := make(map[string][]byte)
	for _, file := range files {
		path := filepath.Join(modulePath, file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s after first generation: %v", file, err)
		}
		firstGenContent[file] = content
	}

	// Second generation (idempotency test)
	if err := generateChildModule([]string{specPath}, "Microsoft.Test/parents/children", modulePath); err != nil {
		t.Fatalf("Second generation failed: %v", err)
	}

	// Verify files are identical
	for _, file := range files {
		path := filepath.Join(modulePath, file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s after second generation: %v", file, err)
		}
		if string(content) != string(firstGenContent[file]) {
			t.Errorf("File %s changed after second generation.\nFirst:\n%s\n\nSecond:\n%s", file, firstGenContent[file], content)
		}
	}
}
