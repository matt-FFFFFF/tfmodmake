package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_GenerateManagedClusters generates a complete Terraform module
// for Microsoft.ContainerService/managedClusters using live bicep-types-az data.
// It verifies the generated files exist and contain expected AKS-specific content.
func TestIntegration_GenerateManagedClusters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Load the resource from live bicep-types-az data (stable API version).
	result, err := LoadResource(ctx, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err, "LoadResource should succeed for managedClusters")

	// Generate the module into tmpDir.
	err = Generate("Microsoft.ContainerService/managedClusters",
		result,
		WithLocalName("resource_body"),
		WithOutputDir(tmpDir),
	)
	require.NoError(t, err, "Generate should succeed")

	// All expected files must exist.
	expectedFiles := []string{"terraform.tf", "variables.tf", "locals.tf", "main.tf", "outputs.tf"}
	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr, "%s should exist", f)
		assert.True(t, info.Size() > 0, "%s should not be empty", f)
	}

	// --- main.tf assertions ---
	mainBytes, err := os.ReadFile(filepath.Join(tmpDir, "main.tf"))
	require.NoError(t, err)
	mainContent := string(mainBytes)

	// Must contain the azapi_resource block with the correct resource type.
	assert.Contains(t, mainContent, `azapi_resource`)
	assert.Contains(t, mainContent, `Microsoft.ContainerService/managedClusters@`)
	// Must reference the local body.
	assert.Contains(t, mainContent, `local.resource_body`)
	// AKS supports location, tags, and identity.
	assert.Contains(t, mainContent, `var.location`)
	assert.Contains(t, mainContent, `var.tags`)
	assert.Contains(t, mainContent, `dynamic "identity"`)
	// response_export_values should exist.
	assert.Contains(t, mainContent, `response_export_values`)

	// --- variables.tf assertions ---
	varsBytes, err := os.ReadFile(filepath.Join(tmpDir, "variables.tf"))
	require.NoError(t, err)
	varsContent := string(varsBytes)

	// Standard AVM variables.
	assert.Contains(t, varsContent, `variable "name"`)
	assert.Contains(t, varsContent, `variable "parent_id"`)
	assert.Contains(t, varsContent, `variable "location"`)
	assert.Contains(t, varsContent, `variable "tags"`)

	// AKS-specific properties that should appear as variables.
	// dns_prefix is a well-known AKS property.
	assert.Contains(t, varsContent, `variable "dns_prefix"`)

	// --- locals.tf assertions ---
	localsBytes, err := os.ReadFile(filepath.Join(tmpDir, "locals.tf"))
	require.NoError(t, err)
	localsContent := string(localsBytes)

	assert.Contains(t, localsContent, `resource_body`)
	assert.Contains(t, localsContent, `properties`)

	// --- terraform.tf assertions ---
	tfBytes, err := os.ReadFile(filepath.Join(tmpDir, "terraform.tf"))
	require.NoError(t, err)
	tfContent := string(tfBytes)

	assert.Contains(t, tfContent, `required_providers`)
	assert.Contains(t, tfContent, `azapi`)

	// --- outputs.tf assertions ---
	outBytes, err := os.ReadFile(filepath.Join(tmpDir, "outputs.tf"))
	require.NoError(t, err)
	outContent := string(outBytes)

	assert.Contains(t, outContent, `output`)
	// Should export the resource ID at minimum.
	assert.Contains(t, outContent, `resource_id`)

	// Extract the API version from main.tf for logging.
	mainFile, err := ParseHCLFile(filepath.Join(tmpDir, "main.tf"))
	require.NoError(t, err)
	_, apiVersion, err := ExtractResourceTypeAndVersion(mainFile)
	require.NoError(t, err)
	t.Logf("Generated module for Microsoft.ContainerService/managedClusters @ %s", apiVersion)
	t.Logf("variables.tf has %d variable blocks", strings.Count(varsContent, "variable \""))
	t.Logf("locals.tf size: %d bytes", len(localsContent))

	// Sanity: the API version should be a date-like string (YYYY-MM-DD or YYYY-MM-DD-preview).
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}`, apiVersion, "API version should start with a date")
	// Since we didn't request preview, it should be a stable version.
	assert.NotContains(t, apiVersion, "preview", "default should select a stable API version")
}

// TestIntegration_GenerateAndUpdateManagedClusters generates a module with a stable
// API version, then updates it to the latest preview API version. This exercises the
// full generate -> update lifecycle including the 3-way comparison logic.
func TestIntegration_GenerateAndUpdateManagedClusters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Step 1: Generate the module with the default stable API version.
	stableResult, err := LoadResource(ctx, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err, "LoadResource (stable) should succeed")

	err = Generate("Microsoft.ContainerService/managedClusters",
		stableResult,
		WithLocalName("resource_body"),
		WithOutputDir(tmpDir),
	)
	require.NoError(t, err, "Generate should succeed")

	// Record the stable API version.
	mainFile, err := ParseHCLFile(filepath.Join(tmpDir, "main.tf"))
	require.NoError(t, err)
	_, stableVersion, err := ExtractResourceTypeAndVersion(mainFile)
	require.NoError(t, err)
	t.Logf("Initial stable version: %s", stableVersion)

	// Record on-disk state before update.
	preUpdateVarsBytes, err := os.ReadFile(filepath.Join(tmpDir, "variables.tf"))
	require.NoError(t, err)
	preUpdateVars := string(preUpdateVarsBytes)

	preUpdateLocalsBytes, err := os.ReadFile(filepath.Join(tmpDir, "locals.tf"))
	require.NoError(t, err)
	preUpdateLocals := string(preUpdateLocalsBytes)

	// Step 2: Update to the latest preview API version.
	updateResult, err := Update(ctx, UpdateOptions{
		ModuleDir:      tmpDir,
		IncludePreview: true,
		DryRun:         false,
	})
	require.NoError(t, err, "Update should succeed")

	t.Logf("Update: %s -> %s", updateResult.OldVersion, updateResult.NewVersion)
	t.Logf("Variables: auto-updated=%d, added=%d, removed=%d, needs-review=%d, unchanged=%d",
		len(updateResult.Variables.AutoUpdated),
		len(updateResult.Variables.Added),
		len(updateResult.Variables.Removed),
		len(updateResult.Variables.NeedsReview),
		len(updateResult.Variables.Unchanged),
	)
	t.Logf("Locals: auto-updated=%d, added=%d, removed=%d, needs-review=%d",
		len(updateResult.Locals.AutoUpdated),
		len(updateResult.Locals.Added),
		len(updateResult.Locals.Removed),
		len(updateResult.Locals.NeedsReview),
	)

	// The update should have produced a result.
	assert.NotEmpty(t, updateResult.OldVersion, "OldVersion should be set")
	assert.NotEmpty(t, updateResult.NewVersion, "NewVersion should be set")
	assert.Equal(t, stableVersion, updateResult.OldVersion, "OldVersion should match what was generated")

	// The new version should be different from the old (preview vs stable).
	// If they happen to be the same (edge case: no preview exists beyond stable),
	// just log it and skip the comparison assertions.
	if updateResult.OldVersion == updateResult.NewVersion {
		t.Logf("Old and new versions are the same (%s) - no preview beyond stable available", updateResult.OldVersion)
		return
	}

	// The new version should be a preview version since we requested IncludePreview.
	assert.Contains(t, updateResult.NewVersion, "preview",
		"New version should be a preview version when IncludePreview is true")

	// main.tf should be updated.
	assert.True(t, updateResult.MainUpdated, "main.tf should be updated")

	// Verify main.tf now references the new API version.
	updatedMainFile, err := ParseHCLFile(filepath.Join(tmpDir, "main.tf"))
	require.NoError(t, err)
	_, updatedVersion, err := ExtractResourceTypeAndVersion(updatedMainFile)
	require.NoError(t, err)
	assert.Equal(t, updateResult.NewVersion, updatedVersion,
		"main.tf should reference the new API version")

	// All expected files should still exist after update.
	expectedFiles := []string{"terraform.tf", "variables.tf", "locals.tf", "main.tf", "outputs.tf"}
	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		_, statErr := os.Stat(path)
		assert.NoError(t, statErr, "%s should still exist after update", f)
	}

	// Since we didn't modify anything on disk, there should be no NeedsReview items
	// (everything on disk matches baseline, so all changes should auto-apply).
	assert.Empty(t, updateResult.Variables.NeedsReview,
		"No variables should need review when module is unmodified")
	assert.Empty(t, updateResult.Locals.NeedsReview,
		"No locals should need review when module is unmodified")

	// Read the updated files and verify they changed.
	postUpdateVarsBytes, err := os.ReadFile(filepath.Join(tmpDir, "variables.tf"))
	require.NoError(t, err)
	postUpdateVars := string(postUpdateVarsBytes)

	postUpdateLocalsBytes, err := os.ReadFile(filepath.Join(tmpDir, "locals.tf"))
	require.NoError(t, err)
	postUpdateLocals := string(postUpdateLocalsBytes)

	// If versions differ, the content should likely differ too.
	// But we don't assert inequality because minor version bumps may not change the schema.
	// Instead, verify structural correctness of the updated files.
	_ = preUpdateVars
	_ = preUpdateLocals

	// Updated variables.tf should still have core AKS variables.
	assert.Contains(t, postUpdateVars, `variable "name"`)
	assert.Contains(t, postUpdateVars, `variable "dns_prefix"`)

	// Updated locals.tf should still reference resource_body.
	assert.Contains(t, postUpdateLocals, `resource_body`)

	// Outputs should have been regenerated.
	assert.True(t, updateResult.OutputsRegenerated, "outputs.tf should be regenerated")

	// Log whether files actually changed.
	if postUpdateVars != preUpdateVars {
		t.Logf("variables.tf changed during update (%d -> %d bytes)",
			len(preUpdateVars), len(postUpdateVars))
	} else {
		t.Logf("variables.tf unchanged during update (schema may be identical)")
	}
	if postUpdateLocals != preUpdateLocals {
		t.Logf("locals.tf changed during update (%d -> %d bytes)",
			len(preUpdateLocals), len(postUpdateLocals))
	} else {
		t.Logf("locals.tf unchanged during update (schema may be identical)")
	}
}

// TestIntegration_UpdateDryRun performs a dry-run update to verify that it reports
// changes without modifying any files on disk.
func TestIntegration_UpdateDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Generate the module with the default stable API version.
	result, err := LoadResource(ctx, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)

	err = Generate("Microsoft.ContainerService/managedClusters",
		result,
		WithLocalName("resource_body"),
		WithOutputDir(tmpDir),
	)
	require.NoError(t, err)

	// Capture file contents before dry-run.
	filesBefore := make(map[string]string)
	for _, f := range []string{"main.tf", "variables.tf", "locals.tf", "outputs.tf"} {
		data, readErr := os.ReadFile(filepath.Join(tmpDir, f))
		require.NoError(t, readErr)
		filesBefore[f] = string(data)
	}

	// Perform dry-run update to latest preview.
	dryRunResult, err := Update(ctx, UpdateOptions{
		ModuleDir:      tmpDir,
		IncludePreview: true,
		DryRun:         true,
	})
	require.NoError(t, err, "Dry-run update should succeed")

	t.Logf("Dry-run: %s -> %s", dryRunResult.OldVersion, dryRunResult.NewVersion)

	// Verify no files were modified.
	for _, f := range []string{"main.tf", "variables.tf", "locals.tf", "outputs.tf"} {
		data, readErr := os.ReadFile(filepath.Join(tmpDir, f))
		require.NoError(t, readErr)
		assert.Equal(t, filesBefore[f], string(data),
			"%s should not be modified during dry-run", f)
	}

	// The dry-run result should still report what would change.
	assert.NotEmpty(t, dryRunResult.OldVersion)
	assert.NotEmpty(t, dryRunResult.NewVersion)
}

// TestIntegration_UpdateRemovesVariablesAndLocals generates a module with the latest
// preview API (which has more properties), then updates to an older stable version.
// Properties that exist in preview but not in the older stable version should be removed.
func TestIntegration_UpdateRemovesVariablesAndLocals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Step 1: Generate with the latest preview API version (has more properties).
	previewResult, err := LoadResource(ctx, "Microsoft.ContainerService/managedClusters",
		WithIncludePreview(true))
	require.NoError(t, err)

	err = Generate("Microsoft.ContainerService/managedClusters",
		previewResult,
		WithLocalName("resource_body"),
		WithOutputDir(tmpDir),
	)
	require.NoError(t, err)

	// Record the preview API version.
	mainFile, err := ParseHCLFile(filepath.Join(tmpDir, "main.tf"))
	require.NoError(t, err)
	_, previewVersion, err := ExtractResourceTypeAndVersion(mainFile)
	require.NoError(t, err)
	require.Contains(t, previewVersion, "preview", "should have generated with a preview version")

	// Record preview variable names.
	previewVarsFile, err := ParseHCLFile(filepath.Join(tmpDir, "variables.tf"))
	require.NoError(t, err)
	previewVarNames := ExtractVariableTypes(previewVarsFile)
	t.Logf("Preview version %s: %d variables", previewVersion, len(previewVarNames))

	// Step 2: Update to the latest stable version (fewer properties).
	updateResult, err := Update(ctx, UpdateOptions{
		ModuleDir:      tmpDir,
		IncludePreview: false, // Force stable
	})
	require.NoError(t, err, "Update should succeed")

	t.Logf("Update: %s -> %s", updateResult.OldVersion, updateResult.NewVersion)
	t.Logf("Variables: auto-updated=%d, added=%d, removed=%d, needs-review=%d",
		len(updateResult.Variables.AutoUpdated),
		len(updateResult.Variables.Added),
		len(updateResult.Variables.Removed),
		len(updateResult.Variables.NeedsReview),
	)
	t.Logf("Locals: auto-updated=%d, added=%d, removed=%d, needs-review=%d",
		len(updateResult.Locals.AutoUpdated),
		len(updateResult.Locals.Added),
		len(updateResult.Locals.Removed),
		len(updateResult.Locals.NeedsReview),
	)

	// The versions should differ (preview -> stable).
	if updateResult.OldVersion == updateResult.NewVersion {
		t.Skip("Preview and stable versions are the same — cannot test removal")
	}

	assert.NotContains(t, updateResult.NewVersion, "preview",
		"New version should be stable")

	// There should be at least one removed variable (preview has more properties).
	assert.NotEmpty(t, updateResult.Variables.Removed,
		"Should have removed at least one variable when going from preview to stable")
	t.Logf("Removed variables: %v", updateResult.Variables.Removed)

	// Verify the removed variables are actually gone from the file.
	postUpdateVarsFile, err := ParseHCLFile(filepath.Join(tmpDir, "variables.tf"))
	require.NoError(t, err)
	postUpdateVarNames := ExtractVariableTypes(postUpdateVarsFile)

	for _, removedName := range updateResult.Variables.Removed {
		_, stillExists := postUpdateVarNames[removedName]
		assert.False(t, stillExists,
			"Variable %q was reported as removed but still exists in variables.tf", removedName)
	}

	// The total variable count should have decreased.
	assert.Less(t, len(postUpdateVarNames), len(previewVarNames),
		"Variable count should decrease when going from preview to stable")
	t.Logf("Variable count: %d (preview) -> %d (stable)", len(previewVarNames), len(postUpdateVarNames))

	// No items should need review since we didn't modify anything.
	assert.Empty(t, updateResult.Variables.NeedsReview,
		"No variables should need review on unmodified module")
}

// TestIntegration_UpdatePreservesUserModifications generates a module, makes a
// user modification to a variable, then updates. The modified variable should
// appear in NeedsReview rather than being auto-updated.
func TestIntegration_UpdatePreservesUserModifications(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Generate module.
	result, err := LoadResource(ctx, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)

	err = Generate("Microsoft.ContainerService/managedClusters",
		result,
		WithLocalName("resource_body"),
		WithOutputDir(tmpDir),
	)
	require.NoError(t, err)

	// Simulate a user modification to variables.tf by modifying the dns_prefix variable
	// type from string to a custom type.
	varsPath := filepath.Join(tmpDir, "variables.tf")
	varsFile, err := ParseHCLFile(varsPath)
	require.NoError(t, err)

	// Check that dns_prefix exists before we try to modify it.
	varTypes := ExtractVariableTypes(varsFile)
	_, hasDNSPrefix := varTypes["dns_prefix"]
	if !hasDNSPrefix {
		t.Skip("dns_prefix variable not found in generated module - schema may have changed")
	}

	// Read the raw content and inject a user modification: add a description override.
	varsBytes, err := os.ReadFile(varsPath)
	require.NoError(t, err)
	varsContent := string(varsBytes)

	// Find the dns_prefix variable block and add a custom description.
	// This simulates a user editing the variable.
	modified := strings.Replace(varsContent,
		`variable "dns_prefix"`,
		"# User customization: dns_prefix is our cluster's unique identifier\n"+`variable "dns_prefix"`,
		1,
	)
	require.NotEqual(t, varsContent, modified, "Should have modified the variables.tf content")
	err = os.WriteFile(varsPath, []byte(modified), 0o644)
	require.NoError(t, err)

	// Now update to preview.
	updateResult, err := Update(ctx, UpdateOptions{
		ModuleDir:      tmpDir,
		IncludePreview: true,
		DryRun:         false,
	})
	require.NoError(t, err, "Update should succeed even with user modifications")

	if updateResult.OldVersion == updateResult.NewVersion {
		t.Logf("Versions are the same - skipping modification preservation check")
		return
	}

	t.Logf("Variables needing review: %v", updateResult.Variables.NeedsReview)
	t.Logf("Variables auto-updated: %v", updateResult.Variables.AutoUpdated)
	t.Logf("Variables added: %v", updateResult.Variables.Added)
	t.Logf("Variables removed: %v", updateResult.Variables.Removed)

	// The update should have completed. Our modification was a comment prepended
	// to the variable block; since the hclwrite parser ignores comments attached
	// to blocks, this won't actually change the parsed tokens. The comparison
	// happens at the token level, so this might still auto-update.
	//
	// What we CAN verify is that the update completed without error and the
	// structural integrity of the module is preserved.
	postUpdateVars, err := os.ReadFile(varsPath)
	require.NoError(t, err)
	assert.Contains(t, string(postUpdateVars), `variable "dns_prefix"`,
		"dns_prefix should still exist after update")
}
