package terraform

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func TestExtractResourceTypeAndVersion(t *testing.T) {
	tests := []struct {
		name        string
		hcl         string
		wantType    string
		wantVersion string
		wantErr     bool
	}{
		{
			name: "standard resource type",
			hcl: `resource "azapi_resource" "this" {
  type = "Microsoft.App/managedEnvironments@2025-10-02-preview"
  name = var.name
}`,
			wantType:    "Microsoft.App/managedEnvironments",
			wantVersion: "2025-10-02-preview",
		},
		{
			name: "stable version",
			hcl: `resource "azapi_resource" "this" {
  type = "Microsoft.ContainerService/managedClusters@2024-02-01"
  name = var.name
}`,
			wantType:    "Microsoft.ContainerService/managedClusters",
			wantVersion: "2024-02-01",
		},
		{
			name: "no azapi_resource this block",
			hcl: `resource "azapi_resource" "other" {
  type = "Microsoft.App/managedEnvironments@2025-10-02-preview"
}`,
			wantErr: true,
		},
		{
			name: "no type attribute",
			hcl: `resource "azapi_resource" "this" {
  name = var.name
}`,
			wantErr: true,
		},
		{
			name:    "nil file",
			hcl:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.hcl == "" && tt.name == "nil file" {
				_, _, err := ExtractResourceTypeAndVersion(nil)
				if !tt.wantErr {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			file, diags := hclwrite.ParseConfig([]byte(tt.hcl), "test.tf", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatalf("failed to parse HCL: %s", diags.Error())
			}

			gotType, gotVersion, err := ExtractResourceTypeAndVersion(file)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType {
				t.Errorf("resource type: got %q, want %q", gotType, tt.wantType)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("version: got %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestSplitTypeAndVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantType    string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "standard",
			input:       "Microsoft.App/managedEnvironments@2025-10-02-preview",
			wantType:    "Microsoft.App/managedEnvironments",
			wantVersion: "2025-10-02-preview",
		},
		{
			name:    "no @ symbol",
			input:   "Microsoft.App/managedEnvironments",
			wantErr: true,
		},
		{
			name:    "empty type",
			input:   "@2025-10-02-preview",
			wantErr: true,
		},
		{
			name:    "empty version",
			input:   "Microsoft.App/managedEnvironments@",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotVersion, err := splitTypeAndVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType {
				t.Errorf("type: got %q, want %q", gotType, tt.wantType)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("version: got %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestExtractVariableTypes(t *testing.T) {
	hclContent := `variable "name" {
  type = string
}

variable "location" {
  type = string
}

variable "config" {
  type = object({
    foo = string
    bar = optional(number)
  })
}
`
	file, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}

	result := ExtractVariableTypes(file)
	if len(result) != 3 {
		t.Fatalf("expected 3 variables, got %d", len(result))
	}

	for _, name := range []string{"name", "location", "config"} {
		if _, ok := result[name]; !ok {
			t.Errorf("expected variable %q in result", name)
		}
	}
}

func TestExtractVariableTypes_nilFile(t *testing.T) {
	result := ExtractVariableTypes(nil)
	if result != nil {
		t.Errorf("expected nil for nil file, got %v", result)
	}
}

func TestExtractLocalAssignments(t *testing.T) {
	hclContent := `locals {
  resource_body = {
    properties = {
      foo = var.foo
    }
  }
  managed_identities = {}
}
`
	file, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}

	result := ExtractLocalAssignments(file)
	if len(result) != 2 {
		t.Fatalf("expected 2 local assignments, got %d", len(result))
	}

	for _, name := range []string{"resource_body", "managed_identities"} {
		if _, ok := result[name]; !ok {
			t.Errorf("expected local %q in result", name)
		}
	}
}

func TestExtractLocalAssignments_nilFile(t *testing.T) {
	result := ExtractLocalAssignments(nil)
	if result != nil {
		t.Errorf("expected nil for nil file, got %v", result)
	}
}

func TestExtractVariableBlocks(t *testing.T) {
	hclContent := `variable "name" {
  type = string
}

variable "location" {
  type = string
  default = null
}
`
	file, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}

	result := ExtractVariableBlocks(file)
	if len(result) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result))
	}
	if _, ok := result["name"]; !ok {
		t.Error("expected variable 'name' in result")
	}
	if _, ok := result["location"]; !ok {
		t.Error("expected variable 'location' in result")
	}
}
