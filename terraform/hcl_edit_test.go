package terraform

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func parseHCL(t *testing.T, src string) *hclwrite.File {
	t.Helper()
	file, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}
	return file
}

func TestUpdateVariableType(t *testing.T) {
	src := `variable "name" {
  type = string
}

variable "location" {
  type = string
}
`
	file := parseHCL(t, src)
	newType := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "number")}

	if err := UpdateVariableType(file, "name", newType); err != nil {
		t.Fatalf("UpdateVariableType() error: %v", err)
	}

	output := string(file.Bytes())
	if !strings.Contains(output, "number") {
		t.Errorf("expected output to contain 'number', got:\n%s", output)
	}
	// location should be unchanged
	if !strings.Contains(output, `variable "location"`) {
		t.Error("expected 'location' variable to remain")
	}
}

func TestUpdateVariableType_notFound(t *testing.T) {
	src := `variable "name" {
  type = string
}
`
	file := parseHCL(t, src)
	newType := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "number")}

	err := UpdateVariableType(file, "nonexistent", newType)
	if err == nil {
		t.Fatal("expected error for missing variable, got nil")
	}
}

func TestAddVariableBlock(t *testing.T) {
	targetSrc := `variable "name" {
  type = string
}
`
	sourceSrc := `variable "location" {
  type    = string
  default = null
}
`
	targetFile := parseHCL(t, targetSrc)
	sourceFile := parseHCL(t, sourceSrc)

	if err := AddVariableBlock(targetFile, sourceFile, "location"); err != nil {
		t.Fatalf("AddVariableBlock() error: %v", err)
	}

	output := string(targetFile.Bytes())
	if !strings.Contains(output, `variable "location"`) {
		t.Errorf("expected output to contain 'location' variable, got:\n%s", output)
	}
	if !strings.Contains(output, `variable "name"`) {
		t.Error("expected original 'name' variable to remain")
	}
}

func TestAddVariableBlock_notFoundInSource(t *testing.T) {
	targetFile := parseHCL(t, `variable "name" { type = string }`)
	sourceFile := parseHCL(t, `variable "other" { type = string }`)

	err := AddVariableBlock(targetFile, sourceFile, "missing")
	if err == nil {
		t.Fatal("expected error for missing variable in source, got nil")
	}
}

func TestRemoveVariableBlock(t *testing.T) {
	src := `variable "name" {
  type = string
}

variable "location" {
  type = string
}
`
	file := parseHCL(t, src)

	if err := RemoveVariableBlock(file, "name"); err != nil {
		t.Fatalf("RemoveVariableBlock() error: %v", err)
	}

	output := string(file.Bytes())
	if strings.Contains(output, `variable "name"`) {
		t.Errorf("expected 'name' variable to be removed, got:\n%s", output)
	}
	if !strings.Contains(output, `variable "location"`) {
		t.Error("expected 'location' variable to remain")
	}
}

func TestRemoveVariableBlock_notFound(t *testing.T) {
	file := parseHCL(t, `variable "name" { type = string }`)

	err := RemoveVariableBlock(file, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing variable, got nil")
	}
}

func TestUpdateLocalAttribute(t *testing.T) {
	src := `locals {
  resource_body = {
    properties = {
      foo = var.foo
    }
  }
  other = "keep"
}
`
	file := parseHCL(t, src)
	newExpr := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, `{ updated = true }`)}

	if err := UpdateLocalAttribute(file, "resource_body", newExpr); err != nil {
		t.Fatalf("UpdateLocalAttribute() error: %v", err)
	}

	output := string(file.Bytes())
	if !strings.Contains(output, "updated") {
		t.Errorf("expected output to contain 'updated', got:\n%s", output)
	}
	if !strings.Contains(output, "other") {
		t.Error("expected 'other' local to remain")
	}
}

func TestUpdateLocalAttribute_notFound(t *testing.T) {
	src := `locals {
  foo = "bar"
}
`
	file := parseHCL(t, src)
	newExpr := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "true")}

	err := UpdateLocalAttribute(file, "nonexistent", newExpr)
	if err == nil {
		t.Fatal("expected error for missing local attribute, got nil")
	}
}

func TestAddLocalAttribute(t *testing.T) {
	src := `locals {
  existing = "value"
}
`
	file := parseHCL(t, src)
	newExpr := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, `"new_value"`)}

	if err := AddLocalAttribute(file, "new_attr", newExpr); err != nil {
		t.Fatalf("AddLocalAttribute() error: %v", err)
	}

	output := string(file.Bytes())
	if !strings.Contains(output, "new_attr") {
		t.Errorf("expected output to contain 'new_attr', got:\n%s", output)
	}
	if !strings.Contains(output, "existing") {
		t.Error("expected 'existing' local to remain")
	}
}

func TestAddLocalAttribute_noLocalsBlock(t *testing.T) {
	src := `variable "name" {
  type = string
}
`
	file := parseHCL(t, src)
	expr := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "true")}

	err := AddLocalAttribute(file, "foo", expr)
	if err == nil {
		t.Fatal("expected error when no locals block exists, got nil")
	}
}

func TestRemoveLocalAttribute(t *testing.T) {
	src := `locals {
  keep_me  = "yes"
  remove_me = "no"
}
`
	file := parseHCL(t, src)

	if err := RemoveLocalAttribute(file, "remove_me"); err != nil {
		t.Fatalf("RemoveLocalAttribute() error: %v", err)
	}

	output := string(file.Bytes())
	if strings.Contains(output, "remove_me") {
		t.Errorf("expected 'remove_me' to be removed, got:\n%s", output)
	}
	if !strings.Contains(output, "keep_me") {
		t.Error("expected 'keep_me' to remain")
	}
}

func TestRemoveLocalAttribute_notFound(t *testing.T) {
	src := `locals {
  foo = "bar"
}
`
	file := parseHCL(t, src)

	err := RemoveLocalAttribute(file, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing local attribute, got nil")
	}
}

func TestUpdateResourceTypeAttribute(t *testing.T) {
	src := `resource "azapi_resource" "this" {
  type = "Microsoft.App/managedEnvironments@2024-01-01"
  name = var.name
}
`
	file := parseHCL(t, src)

	if err := UpdateResourceTypeAttribute(file, "Microsoft.App/managedEnvironments@2025-10-02-preview"); err != nil {
		t.Fatalf("UpdateResourceTypeAttribute() error: %v", err)
	}

	output := string(file.Bytes())
	if !strings.Contains(output, "2025-10-02-preview") {
		t.Errorf("expected output to contain new version, got:\n%s", output)
	}
	if strings.Contains(output, "2024-01-01") {
		t.Error("expected old version to be replaced")
	}
}

func TestUpdateResourceTypeAttribute_notFound(t *testing.T) {
	src := `resource "azapi_resource" "other" {
  type = "Microsoft.App/managedEnvironments@2024-01-01"
}
`
	file := parseHCL(t, src)

	err := UpdateResourceTypeAttribute(file, "Microsoft.App/managedEnvironments@2025-10-02-preview")
	if err == nil {
		t.Fatal("expected error when no azapi_resource 'this' block exists, got nil")
	}
}

func TestUpdateResponseExportValues(t *testing.T) {
	src := `resource "azapi_resource" "this" {
  type                   = "Microsoft.App/managedEnvironments@2024-01-01"
  response_export_values = ["properties.provisioningState"]
}
`
	file := parseHCL(t, src)

	newPaths := []string{"properties.provisioningState", "properties.defaultDomain"}
	if err := UpdateResponseExportValues(file, newPaths); err != nil {
		t.Fatalf("UpdateResponseExportValues() error: %v", err)
	}

	output := string(file.Bytes())
	if !strings.Contains(output, "defaultDomain") {
		t.Errorf("expected output to contain 'defaultDomain', got:\n%s", output)
	}
}

func TestUpdateResponseExportValues_emptyPaths(t *testing.T) {
	src := `resource "azapi_resource" "this" {
  type                   = "Microsoft.App/managedEnvironments@2024-01-01"
  response_export_values = ["properties.provisioningState"]
}
`
	file := parseHCL(t, src)

	if err := UpdateResponseExportValues(file, nil); err != nil {
		t.Fatalf("UpdateResponseExportValues() error: %v", err)
	}

	output := string(file.Bytes())
	if strings.Contains(output, "response_export_values") {
		t.Errorf("expected response_export_values to be removed, got:\n%s", output)
	}
}

func TestUpdateResponseExportValues_notFound(t *testing.T) {
	src := `resource "azapi_resource" "other" {
  type = "Microsoft.App/managedEnvironments@2024-01-01"
}
`
	file := parseHCL(t, src)

	err := UpdateResponseExportValues(file, []string{"foo"})
	if err == nil {
		t.Fatal("expected error when no azapi_resource 'this' block exists, got nil")
	}
}
